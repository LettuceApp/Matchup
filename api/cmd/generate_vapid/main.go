// generate_vapid prints a fresh VAPID keypair for Web Push. Run once
// per environment and paste the output into your .env / secrets store:
//
//	go run ./cmd/generate_vapid
//
// Output format is .env-ready — three lines you can copy-paste into
// the file directly. `VAPID_SUBJECT` is your contact URL (mailto: is
// standard); we default to a placeholder so the block works as-is.
//
// The private key must stay secret. Committing it to git or logging
// it in plaintext breaks the security model — anyone with it can
// impersonate the application server and push notifications to your
// users. Treat it like any other signing key.
//
// A keypair rotation invalidates every existing push subscription:
// the browser's push service tied each endpoint to the public key
// that subscribed it, and refuses deliveries signed with a new one.
// So only rotate when you truly need to.
package main

import (
	"fmt"
	"os"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate_vapid: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("# Add these to your .env (or secrets manager).")
	fmt.Println("# VAPID_SUBJECT must be a mailto: URL or https:// contact.")
	fmt.Printf("VAPID_PUBLIC_KEY=%s\n", pub)
	fmt.Printf("VAPID_PRIVATE_KEY=%s\n", priv)
	fmt.Println("VAPID_SUBJECT=mailto:admin@matchup.com")
}
