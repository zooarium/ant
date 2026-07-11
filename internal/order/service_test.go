package order

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ant/pkg/keeper"
)

// stubRepo embeds the interface so only the methods a test needs are real;
// anything else panics if unexpectedly invoked.
type stubRepo struct {
	Repository
	created *Order // captures the item passed to CreatePublic
}

func (s stubRepo) CreatePublic(ctx context.Context, item Order, items []OrderItemRequest, groupID *int, groupLabel string, maxOrders int, window time.Duration) (Order, error) {
	*s.created = item
	return item, nil
}

func publicReq() CreatePublicOrderRequest {
	return CreatePublicOrderRequest{
		CreateOrderRequest: CreateOrderRequest{
			CustomerName:    "Guest",
			CustomerContact: "9876543210",
			Products:        []OrderItemRequest{{ProductID: 1, Quantity: 1}},
		},
	}
}

func TestCreatePublicRejectsTaxPercent(t *testing.T) {
	svc := NewService(stubRepo{}, nil)
	tax := 5.0
	req := publicReq()
	req.TaxPercent = &tax
	_, err := svc.CreatePublic(context.Background(), 1, 1, 1, "127.0.0.1", req, 0, time.Minute)
	if !errors.Is(err, ErrTaxPercentNotAllowed) {
		t.Fatalf("want ErrTaxPercentNotAllowed, got %v", err)
	}
}

func TestCreatePublicFillsTaxFromAppProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":1,"tax_percent":18}}`))
	}))
	defer srv.Close()

	kc := keeper.NewClient(srv.Client(), srv.URL, time.Minute)
	var created Order
	svc := NewService(stubRepo{created: &created}, kc)

	if _, err := svc.CreatePublic(context.Background(), 1, 1, 1, "127.0.0.1", publicReq(), 0, time.Minute); err != nil {
		t.Fatalf("CreatePublic: %v", err)
	}
	if created.TaxPercent != 18 {
		t.Fatalf("want tax 18, got %v", created.TaxPercent)
	}
}
