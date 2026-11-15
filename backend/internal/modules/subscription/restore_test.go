package subscription

import (
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/testutil"
)

func playPurchase(token string) map[string]any {
	return map[string]any{"store": "google_play", "productId": "plus_android", "purchaseToken": token}
}

func TestRestoreValidation(t *testing.T) {
	e := setup(t, map[string]any{"premium.restore_max_purchases": 2})
	r := e.user(42)
	parse(t, testutil.Request(r, "POST", "/v1/premium/purchases/restore", map[string]any{"purchases": []any{}}, nil), 422)
	env := parse(t, testutil.Request(r, "POST", "/v1/premium/purchases/restore",
		map[string]any{"purchases": []any{playPurchase("a"), playPurchase("b"), playPurchase("c")}}, nil), 422)
	if env.Error.Fields["purchases"] != "must have 1 to 2 items" {
		t.Fatalf("fields %v", env.Error.Fields)
	}
}

func TestRestoreSkipsOtherAccountsPurchase(t *testing.T) {
	e := setup(t, nil)
	e.play.sub = e.activePlay(99) // bought by another Athlix account
	e.mock.ExpectQuery(q("FROM subscriptions")).WillReturnRows(subRows())
	e.mock.ExpectQuery(q("SELECT b.benefit_key, b.value")).WillReturnRows(sqlmock.NewRows([]string{"benefit_key", "value"}))
	env := parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/purchases/restore",
		map[string]any{"purchases": []any{playPurchase("tok-9")}}, nil), 200)
	var out RestoreResult
	_ = json.Unmarshal(env.Data, &out)
	if out.Restored != 0 || out.NotRestored != 1 || out.Subscription.Active != nil {
		t.Fatalf("out %s", env.Data)
	}
	if len(e.events) != 0 {
		t.Fatal("nothing may be activated")
	}
}

func TestRestoreStoreDownIs502(t *testing.T) {
	e := setup(t, nil)
	e.play.err = errUpstream
	parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/purchases/restore",
		map[string]any{"purchases": []any{playPurchase("a"), playPurchase("b")}}, nil), 502)
}

func TestRestoreRelinksOwnPurchase(t *testing.T) {
	e := setup(t, nil)
	e.play.sub = e.activePlay(42)
	h := crypto.SHA256("tok-1")
	e.mock.ExpectQuery(q("WHERE store_product_id_android = ?")).WithArgs("plus_android").WillReturnRows(addPlan(planRows(), 1, true))
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("WHERE store = ? AND store_transaction_id = ? FOR UPDATE")).WithArgs("google_play", hexs(h)).WillReturnRows(subRows())
	e.mock.ExpectExec(q("INSERT INTO subscriptions")).WillReturnResult(sqlmock.NewResult(7, 1))
	e.mock.ExpectCommit()
	e.expectRender(1)
	e.mock.ExpectQuery(q("WHERE user_id = ? AND status IN ('active','grace') AND expires_at > ?")).WithArgs(int64(42), fixedNow).
		WillReturnRows(subRows().AddRow(7, 42, 1, "google_play", hexs(h), "active", true, fixedNow, e.play.sub.ExpiresAt))
	e.expectRender(1)
	e.mock.ExpectQuery(q("SELECT b.benefit_key, b.value")).WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"benefit_key", "value"}))
	env := parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/purchases/restore",
		map[string]any{"purchases": []any{playPurchase("tok-1")}}, nil), 200)
	var out RestoreResult
	_ = json.Unmarshal(env.Data, &out)
	if out.Restored != 1 || out.Subscription.Active == nil || out.Subscription.Active.ID != 7 {
		t.Fatalf("out %s", env.Data)
	}
}
