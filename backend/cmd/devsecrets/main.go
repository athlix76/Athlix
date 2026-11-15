// Command devsecrets writes backend/.secrets.local.json with RANDOM development keys
// (`make dev-secrets`). The file is gitignored. It never contains real provider credentials;
// providers default to stub/mock in app_config, so none are needed to run locally.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func key(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func main() {
	out := flag.String("out", ".secrets.local.json", "output file")
	prefix := flag.String("prefix", "athlix/local", "secret name prefix")
	force := flag.Bool("force", false, "overwrite an existing file")
	flag.Parse()
	if _, err := os.Stat(*out); err == nil && !*force {
		fmt.Println(*out, "already exists (use -force to regenerate; existing encrypted local data becomes unreadable)")
		return
	}
	p := *prefix + "/"
	data := map[string]any{
		p + "jwt":      map[string]string{"userSigningKey": key(32), "adminSigningKey": key(32)},
		p + "pii-keys": map[string]any{"current": 1, "keys": map[string]string{"1": key(32)}, "hmacKey": key(32)},
	}
	b, _ := json.MarshalIndent(data, "", "  ")
	if err := os.WriteFile(*out, b, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", *out)
}
