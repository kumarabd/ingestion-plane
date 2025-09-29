# miner_service.py
import hashlib
import json
import logging
import os
import re
import time
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from typing import List, Tuple, Optional, Dict

import grpc
import redis

# Drain3 imports
from drain3 import TemplateMiner
from drain3.template_miner_config import TemplateMinerConfig

from contracts.miner.v1 import miner_pb2 as pb
from contracts.miner.v1 import miner_pb2_grpc as pb_grpc

# ----------------------------
# Configuration
# ----------------------------
REDIS_URL = os.getenv("MINER_REDIS_URL", "redis://localhost:6379/0")
DRAIN_SIM_THRESHOLD = float(os.getenv("DRAIN_SIM_THRESHOLD", "0.4"))
DRAIN_MAX_DEPTH = int(os.getenv("DRAIN_MAX_DEPTH", "4"))
EXEMPLARS_MAX = int(os.getenv("EXEMPLARS_MAX", "3"))
DEFAULT_CONFIDENCE_MATCH = float(os.getenv("DEFAULT_CONFIDENCE_MATCH", "0.9"))
DEFAULT_CONFIDENCE_NEW = float(os.getenv("DEFAULT_CONFIDENCE_NEW", "0.5"))
LOG_LEVEL = os.getenv("LOG_LEVEL", "DEBUG").upper()

# Setup logging
logging.basicConfig(
    level=getattr(logging, LOG_LEVEL, logging.DEBUG),
    format="%(asctime)s %(levelname)s [%(name)s] %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S"
)
logger = logging.getLogger("miner")
logger.setLevel(getattr(logging, LOG_LEVEL, logging.DEBUG))

# Redis key space
# Hash:  tmpl:{tid} -> { template_text, regex, first_seen_unix, last_seen_unix }
# List:  tmpl:{tid}:examples -> bounded list of example messages (redacted)
# String tmpl:{tid}:labels -> json of minimal labels “last known” (optional)
# TTLs are not set (persistent); you can add TTL if desired.

# ----------------------------
# Utility helpers
# ----------------------------

UUID_RE = re.compile(r"\b[0-9a-fA-F]{8}\b-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-\b[0-9a-fA-F]{12}\b")
IPV4_RE = re.compile(r"\b(?:\d{1,3}\.){3}\d{1,3}\b")
HEX_RE  = re.compile(r"\b0x[0-9a-fA-F]+\b")
NUM_RE  = re.compile(r"\b\d+\b")

def now_ts() -> int:
    return int(time.time())

def dt_to_pb(dt: Optional[datetime]) -> Optional["google.protobuf.timestamp_pb2.Timestamp"]:
    if dt is None:
        return None
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    from google.protobuf.timestamp_pb2 import Timestamp
    ts = Timestamp()
    ts.FromDatetime(dt.astimezone(timezone.utc))
    return ts

def stable_template_id(template_text: str) -> str:
    # 64-bit (8-byte) hex from sha1(template_text)
    h = hashlib.sha1(template_text.encode("utf-8")).digest()
    return h[:8].hex()

def template_to_regex(template_text: str) -> str:
    """
    Convert Drain template to a practical regex:
    Replace parameter tokens with typed regex if recognizable; otherwise generic.
    Drain3 marks parameters typically with '<*>' or '<NUM>' styles depending on config.
    We additionally type-hint based on common placeholders we see in normalized logs.
    """
    logger.debug(f"Converting template to regex: '{template_text[:50]}...'")
    
    # Heuristic: detect placeholders <*>, <NUM>, <IP>, <UUID> or typed <int>, <ipv4>, <uuid>
    text = re.escape(template_text)

    # Replace escaped placeholders with concrete regexes (order matters)
    replacements = [
        (r"<uuid>", r"[0-9a-fA-F\-]{36}"),
        (r"<UUID>", r"[0-9a-fA-F\-]{36}"),
        (r"<ipv4>", r"(?:\d{1,3}\.){3}\d{1,3}"),
        (r"<IP>", r"(?:\d{1,3}\.){3}\d{1,3}"),
        (r"<int>", r"\d+"),
        (r"<num>", r"\d+"),
        (r"<NUM>", r"\d+"),
        (r"<*>", r".+?"),  # generic catch-all non-greedy
    ]
    for ph, rx in replacements:
        text = text.replace(re.escape(ph), rx)

    # Unescape spaces and equals for readability
    text = text.replace(r"\ ", " ").replace(r"\=", "=")
    
    logger.debug(f"Generated regex: '{text[:100]}...'")
    return text

def group_signature(msg: str) -> str:
    """
    Token-type signature like: "w NUM w ipv4 uuid" to characterize structure.
    """
    logger.debug(f"Generating group signature for: '{msg[:50]}...'")
    tokens = msg.split()
    sig_parts = []
    for t in tokens:
        if UUID_RE.search(t):
            sig_parts.append("uuid")
        elif IPV4_RE.search(t):
            sig_parts.append("ipv4")
        elif HEX_RE.search(t):
            sig_parts.append("hex")
        elif NUM_RE.search(t):
            sig_parts.append("num")
        else:
            sig_parts.append("w")
    signature = " ".join(sig_parts[:20])  # cap length
    logger.debug(f"Generated group signature: '{signature}'")
    return signature

def to_provenance(cache_hit: bool) -> pb.Provenance:
    return pb.PROVENANCE_CACHE if cache_hit else pb.PROVENANCE_HEURISTIC

# ----------------------------
# Redis-backed template memory
# ----------------------------
class TemplateMemory:
    def __init__(self, r: redis.Redis):
        self.r = r
        self.log = logging.getLogger("miner.memory")

    def _hkey(self, tid: str) -> str:
        return f"tmpl:{tid}"

    def _lkey_ex(self, tid: str) -> str:
        return f"tmpl:{tid}:examples"

    def get(self, tid: str) -> Optional[Dict]:
        try:
            hkey = self._hkey(tid)
            self.log.info(f"Getting template from Redis: {hkey}")
            h = self.r.hgetall(hkey)
            if not h:
                self.log.info(f"Template not found in cache: {tid}")
                return None
            # Decode bytes → str
            out = {k.decode(): v.decode() for k, v in h.items()}
            # Parse ints if present
            for k in ("first_seen_unix", "last_seen_unix"):
                if k in out:
                    try:
                        out[k] = int(out[k])
                    except ValueError:
                        pass
            self.log.info(f"Retrieved template from cache: {tid}, template='{out.get('template_text', '')[:50]}...'")
            return out
        except Exception as e:
            self.log.error(f"Error getting template {tid} from Redis: {e}", exc_info=True)
            return None

    def upsert(self, tid: str, template_text: str, regex: str, seen_unix: int):
        try:
            key = self._hkey(tid)
            exists = self.r.exists(key)
            self.log.info(f"Upserting template: {tid}, exists={exists}, template='{template_text[:50]}...'")
            pipe = self.r.pipeline()
            if not exists:
                self.log.info(f"Creating new template entry: {tid}")
                pipe.hset(key, mapping={
                    "template_text": template_text,
                    "regex": regex,
                    "first_seen_unix": seen_unix,
                    "last_seen_unix": seen_unix,
                })
            else:
                self.log.info(f"Updating existing template: {tid}")
                pipe.hset(key, mapping={
                    "template_text": template_text,
                    "regex": regex,
                    "last_seen_unix": seen_unix,
                })
            pipe.execute()
            self.log.info(f"Template upserted successfully: {tid}")
        except Exception as e:
            self.log.error(f"Error upserting template {tid}: {e}", exc_info=True)

    def push_example(self, tid: str, message: str):
        try:
            lkey = self._lkey_ex(tid)
            self.log.info(f"Pushing example for template {tid}: '{message[:50]}...'")
            pipe = self.r.pipeline()
            pipe.lpush(lkey, message)
            pipe.ltrim(lkey, 0, EXEMPLARS_MAX - 1)
            pipe.execute()
            self.log.info(f"Example pushed for template {tid}")
        except Exception as e:
            self.log.error(f"Error pushing example for template {tid}: {e}", exc_info=True)

    def get_examples(self, tid: str) -> List[str]:
        try:
            lkey = self._lkey_ex(tid)
            self.log.info(f"Getting examples for template {tid}")
            vals = self.r.lrange(lkey, 0, EXEMPLARS_MAX - 1)
            examples = [v.decode() for v in vals]
            self.log.info(f"Retrieved {len(examples)} examples for template {tid}")
            return examples
        except Exception as e:
            self.log.error(f"Error getting examples for template {tid}: {e}", exc_info=True)
            return []

# ----------------------------
# Drain3 miner wrapper
# ----------------------------
class Drain3Miner:
    def __init__(self):
        self.log = logging.getLogger("miner.drain3")
        self.log.info(f"Initializing Drain3 with sim_threshold={DRAIN_SIM_THRESHOLD}, max_depth={DRAIN_MAX_DEPTH}")
        cfg = TemplateMinerConfig()
        # Set configuration parameters directly
        cfg.drain_sim_th = DRAIN_SIM_THRESHOLD
        cfg.drain_max_depth = DRAIN_MAX_DEPTH
        cfg.drain_max_children = 100
        cfg.drain_max_clusters = 1000
        cfg.profiling_enabled = False
        # Persisting Drain clusters to disk can be added; here we keep in-memory miner
        self.miner = TemplateMiner(config=cfg)
        self.log.info("Drain3 miner initialized successfully")

    def process(self, message: str) -> Tuple[str, bool]:
        """
        Returns (canonical_template, is_match_existing)
        """
        try:
            self.log.info(f"Processing message: '{message[:100]}...'")
            r = self.miner.add_log_message(message)
            # r example: {'change_type': 'cluster_created|cluster_template_changed|none', 'cluster_id': X, 'template_mined': 'text ...'}
            templ = r.get("template_mined") or message
            is_match = r.get("change_type") in ("none", "cluster_template_changed")
            change_type = r.get("change_type", "unknown")
            cluster_id = r.get("cluster_id", "unknown")
            
            self.log.info(f"Drain3 result: change_type={change_type}, cluster_id={cluster_id}, "
                          f"template='{templ[:50]}...', is_match={is_match}")
            
            if change_type == "cluster_created":
                self.log.info(f"New cluster created: {cluster_id} for template '{templ[:50]}...'")
            elif change_type == "cluster_template_changed":
                self.log.info(f"Cluster template updated: {cluster_id} to '{templ[:50]}...'")
            
            return templ, is_match
        except Exception as e:
            self.log.error(f"Drain3 processing failed for message '{message[:50]}...': {e}", exc_info=True)
            # Return the original message as template with low confidence
            return message, False

# ----------------------------
# gRPC Miner Service
# ----------------------------
class MinerService(pb_grpc.MinerServiceServicer):
    def __init__(self, redis_url: str):
        self.log = logging.getLogger("miner.service")
        self.log.info(f"Initializing MinerService with Redis URL: {redis_url}")
        self.redis = redis.from_url(redis_url, decode_responses=False)
        self.mem = TemplateMemory(self.redis)
        self.drain = Drain3Miner()
        self.log.info("MinerService initialized successfully")
        self.log.info(f"Available methods: {[method for method in dir(self) if not method.startswith('_')]}")

    def Analyze(self, request: pb.AnalyzeRequest, context) -> pb.AnalyzeResponse:
        try:
            self.log.info(f"=== ANALYZE REQUEST RECEIVED ===")
            self.log.info(f"Processing AnalyzeRequest with {len(request.records)} records")
            self.log.info(f"Request details: request type={type(request)}, context={context}")
            
            # Log each record in detail
            for idx, rec in enumerate(request.records):
                self.log.info(f"Record {idx}: message='{rec.message}', timestamp={rec.timestamp}, labels={getattr(rec, 'labels', {})}")
            
            results: List[pb.TemplateResult] = []
            shadows: List[pb.MinerShadow] = []  # empty for Drain3 baseline
        
            for idx, rec in enumerate(request.records):
                try:
                    self.log.info(f"Processing record {idx}: message='{rec.message[:100]}...'")
                    # rec is ingest.v1.NormalizedLog (already redacted)
                    msg = rec.message
                    if not msg or not msg.strip():
                        self.log.warning(f"Record {idx} has empty message, skipping")
                        continue
                        
                    ts = rec.timestamp.ToDatetime().replace(tzinfo=timezone.utc) if rec.timestamp.seconds or rec.timestamp.nanos else datetime.now(timezone.utc)

                    # 1) Drain3 mining
                    self.log.info(f"Running Drain3 mining on message: '{msg[:50]}...'")
                    canonical, matched = self.drain.process(msg)
                    self.log.info(f"Drain3 result: canonical='{canonical[:50]}...', matched={matched}")

                    # 2) Build stable template_id and regex
                    tid = stable_template_id(canonical)
                    self.log.info(f"Generated template ID: {tid}")

                    # 3) Read from Redis cache
                    cache_entry = self.mem.get(tid)
                    cache_hit = cache_entry is not None
                    self.log.info(f"Cache lookup for {tid}: hit={cache_hit}")

                    if cache_hit:
                        self.log.info(f"Cache hit for {tid}, using cached template")
                        template_text = cache_entry.get("template_text", canonical)
                        regex = cache_entry.get("regex", template_to_regex(template_text))
                        first_seen_unix = cache_entry.get("first_seen_unix", now_ts())
                        # update last_seen
                        self.mem.upsert(tid, template_text, regex, int(ts.timestamp()))
                    else:
                        self.log.info(f"Cache miss for {tid}, creating new template")
                        template_text = canonical
                        regex = template_to_regex(template_text)
                        self.mem.upsert(tid, template_text, regex, int(ts.timestamp()))

                    # 4) Push a bounded exemplar (already redacted NormalizedLog.message)
                    self.log.info(f"Pushing example for template {tid}")
                    self.mem.push_example(tid, msg)
                    examples = self.mem.get_examples(tid)
                    self.log.info(f"Retrieved {len(examples)} examples for template {tid}")

                    # 5) Build TemplateResult
                    first_seen = None
                    last_seen = None
                    c2 = self.mem.get(tid)
                    if c2 is not None:
                        fsu = c2.get("first_seen_unix", None)
                        lsu = c2.get("last_seen_unix", None)
                        if isinstance(fsu, int):
                            first_seen = dt_to_pb(datetime.fromtimestamp(fsu, tz=timezone.utc))
                        if isinstance(lsu, int):
                            last_seen = dt_to_pb(datetime.fromtimestamp(lsu, tz=timezone.utc))

                    confidence = DEFAULT_CONFIDENCE_MATCH if matched else DEFAULT_CONFIDENCE_NEW
                    provenance = to_provenance(cache_hit)
                    
                    self.log.info(f"Creating TemplateResult for record {idx}: "
                                  f"tid={tid}, confidence={confidence}, provenance={provenance}, "
                                  f"template='{template_text[:50]}...'")
                    
                    result = pb.TemplateResult(
                        record_index=idx,
                        template_id=tid,
                        template=template_text,
                        regex=regex,
                        group_signature=group_signature(msg),
                        confidence=confidence,
                        provenance=provenance,
                        first_seen=first_seen if first_seen else dt_to_pb(ts),
                        last_seen=last_seen if last_seen else dt_to_pb(ts),
                        examples=examples,
                    )
                    results.append(result)
                    self.log.info(f"Added result {idx} to results list. Total results so far: {len(results)}")

                    # 6) No alternates for baseline Drain3; leave shadows empty
                    # If you extend Drain to produce near-miss candidates, append here.
                    
                except Exception as e:
                    self.log.error(f"Error processing record {idx}: {e}", exc_info=True)
                    # Continue processing other records
                    continue

            self.log.info(f"Analysis complete: {len(results)} results, {len(shadows)} shadows")
            self.log.info(f"Results details: {[f'idx={r.record_index}, tid={r.template_id}, template={r.template[:50]}...' for r in results]}")
            
            response = pb.AnalyzeResponse(results=results, shadows=shadows)
            self.log.info(f"Returning response with {len(response.results)} results and {len(response.shadows)} shadows")
            return response
                
        except Exception as e:
            self.log.error(f"Error in Analyze method: {e}", exc_info=True)
            # Return empty response on error
            return pb.AnalyzeResponse(results=[], shadows=[])

# ----------------------------
# Server bootstrap
# ----------------------------
def serve(bind: str = "[::]:50051"):
    logger.info(f"Starting gRPC server on {bind}")
    logger.info(f"Configuration: Redis={REDIS_URL}, Drain3 sim_threshold={DRAIN_SIM_THRESHOLD}, "
                f"max_depth={DRAIN_MAX_DEPTH}, exemplars_max={EXEMPLARS_MAX}")
    
    server = grpc.server(
        ThreadPoolExecutor(max_workers=8),
        options=[
            ("grpc.max_send_message_length", 50 * 1024 * 1024),
            ("grpc.max_receive_message_length", 50 * 1024 * 1024),
        ],
    )
    
    logger.info("Creating MinerService instance")
    service = MinerService(REDIS_URL)
    logger.info("Adding MinerService to gRPC server")
    pb_grpc.add_MinerServiceServicer_to_server(service, server)
    logger.info("Adding insecure port to server")
    server.add_insecure_port(bind)
    
    logger.info(f"Starting server on {bind}")
    server.start()
    logger.info(f"Drain3 gRPC server listening on {bind}, redis={REDIS_URL}")
    logger.info("Server is ready to accept requests")
    print(f"[miner] Drain3 gRPC server listening on {bind}, redis={REDIS_URL}")
    
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        logger.info("Received interrupt signal, shutting down server")
        server.stop(grace=5)
        logger.info("Server shutdown complete")

if __name__ == "__main__":
    serve()
