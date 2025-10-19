package vector

import (
	"context"
	"fmt"
	"log"

	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Upserter handles vector operations with Qdrant
type Upserter struct {
	collectionsClient qdrant.CollectionsClient
	pointsClient      qdrant.PointsClient
	collection        string
	dim               int
}

// NewUpserter creates a new vector upserter
func NewUpserter(ctx context.Context, host string, port int, collection string, dim int) (*Upserter, error) {
	conn, err := grpc.Dial(fmt.Sprintf("%s:%d", host, port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("qdrant connect: %w", err)
	}
	collectionsClient := qdrant.NewCollectionsClient(conn)
	pointsClient := qdrant.NewPointsClient(conn)
	v := &Upserter{
		collectionsClient: collectionsClient,
		pointsClient:      pointsClient,
		collection:        collection,
		dim:               dim,
	}
	if err := v.ensureCollection(ctx); err != nil {
		return nil, err
	}
	return v, nil
}

// ensureCollection ensures the Qdrant collection exists
func (v *Upserter) ensureCollection(ctx context.Context) error {
	// Check if collection exists
	collections, err := v.collectionsClient.List(ctx, &qdrant.ListCollectionsRequest{})
	if err != nil {
		return fmt.Errorf("failed to list collections: %w", err)
	}

	// Check if our collection exists
	for _, col := range collections.GetCollections() {
		if col.GetName() == v.collection {
			log.Printf("INFO: Collection %s already exists", v.collection)
			return nil
		}
	}

	// Create collection if it doesn't exist
	log.Printf("INFO: Creating collection %s with vector dimension %d", v.collection, v.dim)

	// Create collection configuration
	config := &qdrant.CreateCollection{
		CollectionName: v.collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(v.dim),
			Distance: qdrant.Distance_Cosine,
		}),
	}

	_, err = v.collectionsClient.Create(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create collection %s: %w", v.collection, err)
	}

	log.Printf("INFO: Successfully created collection %s", v.collection)
	return nil
}

// UpsertTemplate upserts a template vector into Qdrant
func (v *Upserter) UpsertTemplate(ctx context.Context, id string, vec []float32, payload map[string]interface{}) error {
	log.Printf("DEBUG: VecUpserter.UpsertTemplate called - id=%s, vector_len=%d, collection=%s", id, len(vec), v.collection)

	// Convert payload to Qdrant format
	qdrantPayload := make(map[string]*qdrant.Value)
	for k, val := range payload {
		switch v := val.(type) {
		case string:
			qdrantPayload[k] = qdrant.NewValueString(v)
		case int:
			qdrantPayload[k] = qdrant.NewValueInt(int64(v))
		case int64:
			qdrantPayload[k] = qdrant.NewValueInt(v)
		case float64:
			qdrantPayload[k] = qdrant.NewValueDouble(v)
		case bool:
			qdrantPayload[k] = qdrant.NewValueBool(v)
		case map[string]string:
			// Handle map[string]string (like labels)
			converted := make(map[string]interface{})
			for mk, mv := range v {
				converted[mk] = mv
			}
			if structVal, err := qdrant.NewStruct(converted); err == nil {
				qdrantPayload[k] = qdrant.NewValueStruct(structVal)
			} else {
				qdrantPayload[k] = qdrant.NewValueString(fmt.Sprintf("%v", val))
			}
		case map[string]interface{}:
			// Handle nested objects
			if structVal, err := qdrant.NewStruct(v); err == nil {
				qdrantPayload[k] = qdrant.NewValueStruct(structVal)
			} else {
				qdrantPayload[k] = qdrant.NewValueString(fmt.Sprintf("%v", val))
			}
		default:
			// Convert to string as fallback
			qdrantPayload[k] = qdrant.NewValueString(fmt.Sprintf("%v", val))
		}
	}

	// Generate numeric ID from string
	hash := uint64(0)
	for _, b := range []byte(id) {
		hash = hash*31 + uint64(b)
	}

	point := &qdrant.PointStruct{
		Id:      qdrant.NewIDNum(hash),
		Vectors: qdrant.NewVectors(vec...),
		Payload: qdrantPayload,
	}

	// Upsert the point
	upsertReq := &qdrant.UpsertPoints{
		CollectionName: v.collection,
		Points:         []*qdrant.PointStruct{point},
	}

	_, err := v.pointsClient.Upsert(ctx, upsertReq)
	if err != nil {
		return fmt.Errorf("failed to upsert point %s: %w", id, err)
	}

	log.Printf("INFO: Successfully upserted template %s with vector of length %d to collection %s", id, len(vec), v.collection)
	return nil
}
