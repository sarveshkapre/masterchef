package grpcapi

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/masterchef/masterchef/internal/state"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/test/bufconn"
)

func TestServiceHealthAndListRuns(t *testing.T) {
	baseDir := t.TempDir()
	st := state.New(baseDir)
	if err := st.SaveRun(state.RunRecord{
		ID:        "run-1",
		StartedAt: time.Now().UTC().Add(-time.Minute),
		EndedAt:   time.Now().UTC(),
		Status:    state.RunSucceeded,
	}); err != nil {
		t.Fatal(err)
	}

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	New(baseDir).Register(srv)
	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(func() {
		srv.Stop()
		_ = lis.Close()
	})

	ctx := context.Background()
	conn, err := grpc.DialContext(
		ctx,
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(encoding.GetCodec("json"))),
	)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	var health HealthResponse
	if err := conn.Invoke(ctx, "/masterchef.v1.Control/Health", &HealthRequest{}, &health); err != nil {
		t.Fatalf("health invoke failed: %v", err)
	}
	if health.Status != "ok" {
		t.Fatalf("unexpected health response: %+v", health)
	}

	var runs ListRunsResponse
	if err := conn.Invoke(ctx, "/masterchef.v1.Control/ListRuns", &ListRunsRequest{Limit: 10}, &runs); err != nil {
		t.Fatalf("list runs invoke failed: %v", err)
	}
	if runs.Count != 1 || len(runs.Items) != 1 || runs.Items[0].ID != "run-1" {
		t.Fatalf("unexpected runs response: %+v", runs)
	}
}
