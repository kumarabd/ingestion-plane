package vector

import (
	"context"
	"fmt"
	"strings"

	qdrant "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client handles Qdrant operations
type Client struct {
	points     qdrant.PointsClient
	collection string
	vectorDim  int
}

// New creates a new Qdrant client
func New(ctx context.Context, host string, port int, collection string, vectorDim int) (*Client, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("qdrant dial: %w", err)
	}

	return &Client{
		points:     qdrant.NewPointsClient(conn),
		collection: collection,
		vectorDim:  vectorDim,
	}, nil
}

// Search performs ANN search in Qdrant
func (c *Client) Search(ctx context.Context, vec []float32, tenant string, labelFilter map[string]string, topK int) ([]*qdrant.ScoredPoint, error) {
	filter := c.buildFilter(tenant, labelFilter)

	req := &qdrant.SearchPoints{
		CollectionName: c.collection,
		Vector:         vec,
		Limit:          uint64(topK),
		ScoreThreshold: nil,
		Filter:         filter,
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
		Params:         nil,
	}

	resp, err := c.points.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetResult(), nil
}

// buildFilter creates a Qdrant filter for tenant and label filtering
func (c *Client) buildFilter(tenant string, labelFilter map[string]string) *qdrant.Filter {
	must := []*qdrant.Condition{
		// enforce tenant in payload
		hasStringMatch("tenant", tenant),
	}
	allow := map[string]bool{"service": true, "env": true, "namespace": true, "severity": true}
	for k, v := range labelFilter {
		if !allow[k] || strings.TrimSpace(v) == "" {
			continue
		}
		must = append(must, hasStringMatch("labels."+k, v))
	}
	return &qdrant.Filter{Must: must}
}

// hasStringMatch creates a Qdrant string match condition
func hasStringMatch(path, value string) *qdrant.Condition {
	return &qdrant.Condition{
		ConditionOneOf: &qdrant.Condition_Field{
			Field: &qdrant.FieldCondition{
				Key: path,
				Match: &qdrant.Match{
					MatchValue: &qdrant.Match_Text{
						Text: value,
					},
				},
			},
		},
	}
}

// PayloadString extracts string value from Qdrant payload
func PayloadString(pl map[string]*qdrant.Value, key string) string {
	if pl == nil {
		return ""
	}
	if v, ok := pl[key]; ok {
		if vv := v.GetStringValue(); vv != "" {
			return vv
		}
		if arr := v.GetListValue(); arr != nil && len(arr.Values) > 0 {
			// sometimes string stored as 1-element list
			if s := arr.Values[0].GetStringValue(); s != "" {
				return s
			}
		}
	}
	return ""
}
