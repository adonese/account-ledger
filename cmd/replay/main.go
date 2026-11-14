// Command replay prints the daily results for the supplied event stream.
package main

import (
	"log"
	"os"

	ledger "github.com/adonese/account-ledger"
)

func main() {
	l, err := ledger.Replay(ledger.StreamAccounts(), ledger.Stream())
	if err != nil {
		log.Fatal(err)
	}
	l.Report(os.Stdout)
}
