# # miner_service.py
# # Python gRPC Miner service with Redis state and LibreLog integration.
# # Implements the protobuf contract you provided.

# import os
# import re
# import time
# import json
# import hashlib
# import logging
# import threading
# from dataclasses import dataclass
# from typing import List, Tuple, Dict, Optional

# import grpc
# from concurrent import futures

# import redis  # redis-py

# # Import generated protobuf contracts
# from contracts.miner.v1 import miner_pb2
# from contracts.miner.v1 import miner_pb2_grpc
# from google.protobuf import timestamp_pb2
# from google.protobuf.timestamp_pb2 import Timestamp

# from LibreLog import miner as libre_miner


# # ============================ CONFIG ============================

# @dataclass
# class MinerConfig:
#     redis_url: str = os.getenv("REDIS_URL", "redis://localhost:6379/0")
#     # LLM/LibreLog budget per-tenant (calls per minute)
#     llm_qpm: int = int(os.getenv("LLM_QPM", "60"))
#     # Grouping bucket TTL (seconds)
#     bucket_ttl_s: int = int(os.getenv("BUCKET_TTL_S", "300"))
#     # Template cache TTL (seconds) for signature->template mapping
#     template_ttl_s: int = int(os.getenv("TEMPLATE_TTL_S", "3600"))
#     # Confidence defaults
#     cache_confidence: float = 0.98
#     heuristic_confidence: float = 0.6
#     fallback_confidence: float = 0.3
#     # Max exemplars we keep per template in Redis (tiny)
#     exemplars_max: int = int(os.getenv("EXEMPLARS_MAX", "3"))
#     # Batch behavior: free to tune in your caller, here we process request.records as-is
#     # Regex synth placeholders
#     placeholder_map: Dict[str, str] = None

#     def __post_init__(self):
#         if self.placeholder_map is None:
#             self.placeholder_map = {
#                 "<int>": r"\d+",
#                 "<float>": r"[+-]?(?:\d*\.\d+|\d+)",
#                 "<ipv4>": r"(?:\d{1,3}\.){3}\d{1,3}",
#                 "<uuid>": r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}",
#                 "<email>": r"[^@\s]+@[^@\s]+\.[^@\s]+",
#                 "<word>": r"[A-Za-z0-9_\-\.]+",
#                 "<hex>": r"[0-9a-fA-F]+"
#             }


# # ============================ REDIS STATE ============================

# class MinerState:
#     """
#     Redis-backed fast state for template lookup, grouping buckets, and LLM rate-limits.
#     Key design (prefix with tenant where appropriate):
#       - tmpl:by_sig:{tenant}:{sig} -> JSON {template, regex, template_id, confidence, first_seen, last_seen, exemplars[]}
#       - grp:bkt:{tenant}:{mask_sig} -> JSON bucket {exemplars[], size, last_seen}
#       - llm:budget:{tenant}:{yyyyMMddHHmm} -> integer counter (calls in minute)
#     """

#     def __init__(self, cfg: MinerConfig):
#         self.r = redis.from_url(cfg.redis_url, decode_responses=True)
#         self.cfg = cfg

#     # -------- Template cache --------
#     def get_template_by_signature(self, tenant: str, signature: str) -> Optional[dict]:
#         key = f"tmpl:by_sig:{tenant}:{signature}"
#         data = self.r.get(key)
#         return json.loads(data) if data else None

#     def set_template_by_signature(self, tenant: str, signature: str, value: dict):
#         key = f"tmpl:by_sig:{tenant}:{signature}"
#         self.r.setex(key, self.cfg.template_ttl_s, json.dumps(value))

#     # -------- Grouping buckets (for novel signatures) --------
#     def add_to_bucket(self, tenant: str, masked_signature: str, exemplar: str):
#         key = f"grp:bkt:{tenant}:{masked_signature}"
#         bucket = self.r.get(key)
#         if bucket:
#             bucket = json.loads(bucket)
#         else:
#             bucket = {"exemplars": [], "size": 0, "last_seen": int(time.time())}
#         if exemplar:
#             if len(bucket["exemplars"]) < self.cfg.exemplars_max:
#                 bucket["exemplars"].append(exemplar)
#         bucket["size"] += 1
#         bucket["last_seen"] = int(time.time())
#         self.r.setex(key, self.cfg.bucket_ttl_s, json.dumps(bucket))
#         return bucket

#     def get_bucket(self, tenant: str, masked_signature: str) -> Optional[dict]:
#         key = f"grp:bkt:{tenant}:{masked_signature}"
#         v = self.r.get(key)
#         return json.loads(v) if v else None

#     # -------- LLM/LibreLog rate limit (per-minute) --------
#     def allow_llm_call(self, tenant: str) -> bool:
#         minute_key = time.strftime("%Y%m%d%H%M", time.gmtime())
#         key = f"llm:budget:{tenant}:{minute_key}"
#         cnt = self.r.incr(key, amount=1)
#         # expire after 90s to cover minute window
#         self.r.expire(key, 90)
#         allowed = cnt <= self.cfg.llm_qpm
        
#         # Create a logger instance for debug logging
#         import logging
#         logger = logging.getLogger("miner.state")
#         logger.info("LLM budget check for tenant %s: %d/%d calls used, allowed=%s", 
#                     tenant, cnt, self.cfg.llm_qpm, allowed)
        
#         return allowed


# # ============================ LIBRELOG ADAPTER ============================

# class LibreLogClient:
#     """
#     Adapter around LibreLog’s Python API (direct module call).
#     """
#     def __init__(self, cfg: MinerConfig, logger: logging.Logger):
#         self.cfg = cfg
#         self.log = logger

#     def analyze(self, exemplars: List[str], message: str) -> Optional[dict]:
#         try:
#             self.log.debug("LibreLog analyze called with %d exemplars, message_len=%d", 
#                           len(exemplars), len(message))
            
#             # Example: LibreLog miner API
#             # Assuming librelog.miner.mine_template(exemplars, message)
#             res = libre_miner.mine_template(exemplars, message)

#             # Shape the output into what our MinerService expects
#             result = {
#                 "template": res["template"],
#                 "regex": res.get("regex"),
#                 "confidence": float(res.get("confidence", 0.8))
#             }
            
#             self.log.debug("LibreLog analysis successful: template='%s', confidence=%.2f", 
#                           result["template"][:50] + "..." if len(result["template"]) > 50 else result["template"],
#                           result["confidence"])
            
#             return result
#         except Exception as e:
#             self.log.error("LibreLog module call failed: %s", e, exc_info=True)
#             return None

# # ============================ MINER CORE ============================

# def sha1_8(s: str) -> str:
#     return hashlib.sha1(s.encode("utf-8")).hexdigest()[:16]

# TOKEN_PATTERNS = [
#     (re.compile(r"\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}\b"), "<uuid>"),
#     (re.compile(r"\b(?:\d{1,3}\.){3}\d{1,3}\b"), "<ipv4>"),
#     (re.compile(r"\b\d+\b"), "<int>"),
#     # Add more tokenizers as needed (email, hex, float...)
# ]

# def mask_preview(message: str) -> Tuple[str, str]:
#     """
#     Returns (masked_message, group_signature)
#     masked_message replaces dynamic tokens with placeholders.
#     group_signature captures the token sequence (rough bucket id).
#     """
#     masked = message
#     seq = []
#     for patt, ph in TOKEN_PATTERNS:
#         def repl(m):
#             seq.append(ph)
#             return ph
#         masked = patt.sub(repl, masked)
    
#     # sequence/order for grouping (coarse)
#     sig = "token_seq:" + ",".join(seq[:8])
#     return masked, sig


# def synth_regex_from_template(template: str, placeholder_map: Dict[str, str]) -> str:
#     # Escape literal text, then replace placeholders with their regex equivalents
#     # Simple pass: split on placeholders and rebuild
#     parts = re.split(r"(<[^>]+>)", template)
#     out = []
#     for p in parts:
#         if p in placeholder_map:
#             out.append(placeholder_map[p])
#         elif p.startswith("<") and p.endswith(">") and p not in placeholder_map:
#             out.append(r".+?")  # unknown placeholder (rare) → lazy wildcard
#         else:
#             out.append(re.escape(p))
    
#     return "".join(out)


# class MinerService(miner_pb2_grpc.MinerServiceServicer):
#     def __init__(self, cfg: MinerConfig):
#         self.cfg = cfg
#         self.state = MinerState(cfg)
#         self.log = logging.getLogger("miner")
#         self.log.setLevel(logging.INFO)
#         self.librelog = LibreLogClient(cfg, self.log)

#     # ---- gRPC method ----
#     def Analyze(self, request, context):
#         """
#         request: AnalyzeRequest { records: [miner.v1.NormalizedLog] }
#         returns: AnalyzeResponse { results: [TemplateResult], shadows: [MinerShadow] }
#         """
#         results: List[miner_pb2.TemplateResult] = []
#         shadows: List[miner_pb2.MinerShadow] = []

#         # tenant inference (label or default)
#         # We'll per-record derive tenant to allow multi-tenant logic (budgeting).
#         self.log.debug("Processing batch of %d records", len(request.records))
        
#         for idx, rec in enumerate(request.records):
#             try:
#                 # Extract basics
#                 labels = getattr(rec, "labels", {})  # map<string,string>
#                 tenant = labels.get("tenant", "default")
#                 msg = getattr(rec, "message", "")
                
#                 self.log.debug("Processing record %d: tenant=%s, message_len=%d", 
#                               idx, tenant, len(msg) if msg else 0)

#                 # 1) Mask + signature
#                 masked, group_sig = mask_preview(msg)
#                 signature = sha1_8(masked)  # stable key for cache
                
#                 self.log.debug("Record %d: masked='%s', signature=%s, group_sig=%s", 
#                               idx, masked[:100] + "..." if len(masked) > 100 else masked, 
#                               signature, group_sig)

#                 # 2) Fast cache path
#                 cached = self.state.get_template_by_signature(tenant, signature)
#                 if cached:
#                     self.log.debug("Record %d: cache hit for signature %s", idx, signature)
#                     tr = self._result_from_cache(idx, cached, group_sig)
#                     results.append(tr)
#                     continue
#                 else:
#                     self.log.debug("Record %d: cache miss for signature %s", idx, signature)

#                 # 3) Grouping bucket & heuristic
#                 bucket = self.state.add_to_bucket(tenant, masked, masked)
#                 provenance = miner_pb2.Provenance.PROVENANCE_HEURISTIC
                
#                 self.log.debug("Record %d: added to bucket, size=%d", idx, bucket.get("size", 0))

#                 # 4) LLM/LibreLog budgeted refinement
#                 # Only call when allowed; otherwise use heuristic/fallback.
#                 llm_candidate = None
#                 if self.state.allow_llm_call(tenant):
#                     self.log.debug("Record %d: LLM call allowed for tenant %s", idx, tenant)
#                     llm_candidate = self.librelog.analyze(bucket.get("exemplars", []), masked)
#                     if llm_candidate and llm_candidate.get("template"):
#                         self.log.debug("Record %d: LLM analysis successful, template='%s'", 
#                                       idx, llm_candidate.get("template", "")[:50] + "..." 
#                                       if len(llm_candidate.get("template", "")) > 50 else llm_candidate.get("template", ""))
#                     else:
#                         self.log.debug("Record %d: LLM analysis failed or returned no template", idx)
#                 else:
#                     self.log.debug("Record %d: LLM call denied for tenant %s (rate limited)", idx, tenant)
                    
#                 if llm_candidate and llm_candidate.get("template"):
#                         provenance = miner_pb2.Provenance.PROVENANCE_LIBRELOG
#                         template_text = llm_candidate["template"]
#                         regex = llm_candidate.get("regex") or synth_regex_from_template(template_text, self.cfg.placeholder_map)
#                         confidence = float(llm_candidate.get("confidence", self.cfg.heuristic_confidence))
#                         template_id = sha1_8(template_text)
#                         first_seen = int(time.time())
#                         last_seen = first_seen
                        
#                         self.log.debug("Record %d: using LLM result - template_id=%s, confidence=%.2f", 
#                                       idx, template_id, confidence)
                        
#                         # persist in cache for signature
#                         self.state.set_template_by_signature(tenant, signature, {
#                             "template": template_text,
#                             "regex": regex,
#                             "template_id": template_id,
#                             "confidence": confidence,
#                             "first_seen": first_seen,
#                             "last_seen": last_seen,
#                             "exemplars": bucket.get("exemplars", [])[: self.cfg.exemplars_max]
#                         })
#                         tr = self._result_from_dict(idx, template_id, template_text, regex, group_sig,
#                                                     confidence, provenance, first_seen, last_seen,
#                                                     bucket.get("exemplars", [])[:1])
#                         results.append(tr)

#                         # Build a small shadow with heuristic fallback as alternate (optional)
#                         fallback_template = masked
#                         if fallback_template != template_text:
#                             fb_regex = synth_regex_from_template(fallback_template, self.cfg.placeholder_map)
#                             shadow = miner_pb2.MinerShadow()
#                             shadow.record_index = idx
                            
#                             fallback_result = miner_pb2.TemplateResult()
#                             fallback_result.record_index = idx
#                             fallback_result.template_id = sha1_8(fallback_template)
#                             fallback_result.template = fallback_template
#                             fallback_result.regex = fb_regex
#                             fallback_result.group_signature = group_sig
#                             fallback_result.confidence = self.cfg.fallback_confidence
#                             fallback_result.provenance = miner_pb2.Provenance.PROVENANCE_FALLBACK
#                             fallback_result.first_seen.CopyFrom(_ts(first_seen))
#                             fallback_result.last_seen.CopyFrom(_ts(last_seen))
#                             fallback_result.examples.extend(bucket.get("exemplars", [])[:1])
                            
#                             shadow.candidates.append(fallback_result)
#                             shadows.append(shadow)
#                         continue

#                 # 5) Fallback path (no cache, LLM denied/unavailable)
#                 provenance = provenance if llm_candidate else miner_pb2.Provenance.PROVENANCE_FALLBACK
#                 template_text = masked
#                 regex = synth_regex_from_template(template_text, self.cfg.placeholder_map)
#                 confidence = self.cfg.heuristic_confidence if provenance == miner_pb2.Provenance.PROVENANCE_HEURISTIC else self.cfg.fallback_confidence
#                 template_id = sha1_8(template_text)
#                 first_seen = int(time.time())
#                 last_seen = first_seen

#                 self.log.debug("Record %d: using fallback path - provenance=%s, template_id=%s, confidence=%.2f", 
#                               idx, "HEURISTIC" if provenance == miner_pb2.Provenance.PROVENANCE_HEURISTIC else "FALLBACK",
#                               template_id, confidence)

#                 # persist to speed up future identical lines
#                 self.state.set_template_by_signature(tenant, signature, {
#                     "template": template_text,
#                     "regex": regex,
#                     "template_id": template_id,
#                     "confidence": confidence,
#                     "first_seen": first_seen,
#                     "last_seen": last_seen,
#                     "exemplars": bucket.get("exemplars", [])[: self.cfg.exemplars_max]
#                 })

#                 tr = self._result_from_dict(idx, template_id, template_text, regex, group_sig,
#                                             confidence, provenance, first_seen, last_seen,
#                                             bucket.get("exemplars", [])[:1])
#                 results.append(tr)
                
#             except Exception as e:
#                 self.log.error("Failed to process record %d: %s", idx, e, exc_info=True)
#                 # Re-raise the exception instead of creating fallback results
#                 raise

#         self.log.debug("Batch processing complete: %d results, %d shadows", len(results), len(shadows))
#         response = miner_pb2.AnalyzeResponse()
#         response.results.extend(results)
#         response.shadows.extend(shadows)
#         return response

#     # ---- helpers ----

#     def _result_from_cache(self, idx: int, cached: dict, group_sig: str) -> miner_pb2.TemplateResult:
#         result = miner_pb2.TemplateResult()
#         result.record_index = idx
        
#         result.template_id = cached["template_id"]
#         result.template = cached["template"]
#         result.regex = cached["regex"]
        
#         result.group_signature = group_sig
#         result.confidence = float(cached.get("confidence", self.cfg.cache_confidence))
#         result.provenance = miner_pb2.Provenance.PROVENANCE_CACHE
#         result.first_seen.CopyFrom(_ts(int(cached.get("first_seen", int(time.time())))))
#         result.last_seen.CopyFrom(_ts(int(cached.get("last_seen", int(time.time())))))
#         result.examples.extend(cached.get("exemplars", [])[:1])
#         return result

#     def _result_from_dict(
#         self,
#         idx: int,
#         template_id: str,
#         template_text: str,
#         regex: str,
#         group_sig: str,
#         confidence: float,
#         provenance: int,
#         first_seen: int,
#         last_seen: int,
#         examples: List[str],
#     ) -> miner_pb2.TemplateResult:
#         result = miner_pb2.TemplateResult()
#         result.record_index = idx
        
#         result.template_id = template_id
#         result.template = template_text
#         result.regex = regex
        
#         result.group_signature = group_sig
#         result.confidence = confidence
#         result.provenance = provenance
#         result.first_seen.CopyFrom(_ts(first_seen))
#         result.last_seen.CopyFrom(_ts(last_seen))
#         result.examples.extend(examples)
#         return result


# def _ts(unix_s: int) -> Timestamp:
#     t = timestamp_pb2.Timestamp()
#     t.FromSeconds(unix_s)
#     return t


# # ============================ SERVER BOOT ============================

# def serve(port: int = 50051):
#     logging.info("Starting MinerService on port %d", port)
    
#     server = grpc.server(
#         futures.ThreadPoolExecutor(max_workers=8),
#         options=[
#             ("grpc.max_receive_message_length", 32 * 1024 * 1024),
#             ("grpc.max_send_message_length", 32 * 1024 * 1024),
#         ],
#     )
#     cfg = MinerConfig()
    
#     # Log configuration
#     logging.info("MinerService configuration:")
#     logging.info("  Redis URL: %s", cfg.redis_url)
#     logging.info("  LLM QPM: %d", cfg.llm_qpm)
#     logging.info("  Bucket TTL: %d seconds", cfg.bucket_ttl_s)
#     logging.info("  Template TTL: %d seconds", cfg.template_ttl_s)
#     logging.info("  Cache confidence: %.2f", cfg.cache_confidence)
#     logging.info("  Heuristic confidence: %.2f", cfg.heuristic_confidence)
#     logging.info("  Fallback confidence: %.2f", cfg.fallback_confidence)
#     logging.info("  Max exemplars: %d", cfg.exemplars_max)
    
#     svc = MinerService(cfg)
#     miner_pb2_grpc.add_MinerServiceServicer_to_server(svc, server)
#     server.add_insecure_port(f"[::]:{port}")
#     server.start()
#     logging.info("MinerService listening on :%d", port)
#     server.wait_for_termination()


# if __name__ == "__main__":
#     # Set log level from environment variable, default to INFO
#     log_level = os.getenv("LOG_LEVEL", "INFO").upper()
    
#     logging.basicConfig(
#         level=getattr(logging, log_level, logging.INFO),
#         format="%(asctime)s %(levelname)s %(name)s: %(message)s"
#     )
    
#     # Set specific logger levels
#     logging.getLogger("miner").setLevel(getattr(logging, log_level, logging.INFO))
#     logging.getLogger("miner.state").setLevel(getattr(logging, log_level, logging.INFO))
    
#     logging.info("Starting miner with log level: %s", log_level)
#     serve()
