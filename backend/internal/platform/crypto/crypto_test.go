package crypto

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/google/uuid"
)

func testKeys() Keys {
	k1 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	k2 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	h := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	return Keys{Current: 2, Keys: map[string]string{"1": k1, "2": k2}, HMACKey: h}
}

func TestRoundTripAndRotation(t *testing.T) {
	k := testKeys()
	old := k
	old.Current = 1
	oldBox, err := NewBox(old)
	if err != nil {
		t.Fatal(err)
	}
	sealedOld, _ := oldBox.EncryptString("1990-01-01")

	box, err := NewBox(k)
	if err != nil {
		t.Fatal(err)
	}
	sealed, _ := box.EncryptString("hello")
	if got, _ := box.DecryptString(sealed); got != "hello" {
		t.Fatalf("got %q", got)
	}
	if got, _ := box.DecryptString(sealedOld); got != "1990-01-01" {
		t.Fatalf("rotation: got %q", got)
	}
	sealed[len(sealed)-1] ^= 0xff
	if _, err := box.Decrypt(sealed); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
}

func TestRejectsShortKey(t *testing.T) {
	k := testKeys()
	k.Keys["2"] = base64.StdEncoding.EncodeToString([]byte("short"))
	if _, err := NewBox(k); err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestHashPurposeScoped(t *testing.T) {
	box, _ := NewBox(testKeys())
	if bytes.Equal(box.Hash("mobile", "x"), box.Hash("guardian", "x")) {
		t.Fatal("hash must differ per purpose")
	}
}

func TestStoreAccountTokenMatchesUUIDLayout(t *testing.T) {
	b, err := NewBox(testKeys())
	if err != nil {
		t.Fatal(err)
	}
	a := b.StoreAccountToken(42)
	u, err := uuid.Parse(a)
	if err != nil || u.Version() != 4 || u.Variant() != uuid.RFC4122 || u.String() != a {
		t.Fatalf("%q %v", a, err)
	}
	if b.StoreAccountToken(42) != a || b.StoreAccountToken(43) == a {
		t.Fatal("must be stable per user and differ between users")
	}
}
