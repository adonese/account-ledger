// Command bench replays a seeded synthetic stream and prints the median replay
// and report times, the journal length, the heap the journal takes, and a hash
// of the journal and report, so that two builds of the ledger can be compared
// byte for byte. With -report=false the report is skipped and the hash covers
// the journal only. run.sh in this directory reproduces the table in
// DESIGN.md, section 1.
package main

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"runtime/pprof"
	"slices"
	"time"

	ledger "github.com/adonese/account-ledger"
)

func main() {
	events := flag.Int("events", 1000, "number of synthetic events")
	accounts := flag.Int("accounts", 2, "number of synthetic accounts")
	seed := flag.Uint64("seed", 1, "generator seed")
	runs := flag.Int("runs", 1, "replays to take the median of")
	deadline := flag.Duration("deadline", 5*time.Minute, "give up on a replay after this long")
	brief := flag.Bool("brief", false, "replay the brief's stream instead of a synthetic one")
	cpuprofile := flag.String("cpuprofile", "", "write a CPU profile of the first replay to this file")
	withReport := flag.Bool("report", true, "also time the report and include it in the hash")
	flag.Parse()

	accts, evs := ledger.StreamAccounts(), ledger.Stream()
	if !*brief {
		accts, evs = generate(*events, *accounts, *seed)
	}

	var replays, reports []time.Duration
	var last result
	for i := range max(*runs, 1) {
		profile := ""
		if i == 0 {
			profile = *cpuprofile
		}
		r, finished := measure(accts, evs, *deadline, profile, *withReport)
		if !finished {
			fmt.Printf("events=%d accounts=%d replay=unfinished-after-%s\n", len(evs), len(accts), *deadline)
			os.Exit(2)
		}
		if i > 0 && r.hash != last.hash {
			fmt.Fprintln(os.Stderr, "bench: replays of the same stream differ")
			os.Exit(1)
		}
		last = r
		replays, reports = append(replays, r.replay), append(reports, r.report)
	}

	heap := "n/a" // the profiler's own buffers would distort it
	if *cpuprofile == "" {
		heap = fmt.Sprintf("%.2fMB bytes/line=%d", float64(last.heap)/1e6, last.heap/int64(last.lines))
	}
	report := "skipped"
	if *withReport {
		report = median(reports).String()
	}
	fmt.Printf("events=%d accounts=%d lines=%d replay=%s report=%s heap=%s hash=%x\n",
		len(evs), len(accts), last.lines, median(replays), report, heap, last.hash)
}

type result struct {
	replay, report time.Duration
	lines          int
	heap           int64 // bytes held by the ledger after replay, including slice growth
	hash           [8]byte
}

// measure replays once. It returns false if the replay misses the deadline.
func measure(accounts []ledger.Account, events []ledger.Event, deadline time.Duration, profile string, withReport bool) (result, bool) {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	stop := startProfile(profile)

	done := make(chan *ledger.Ledger, 1)
	start := time.Now()
	go func() {
		l, err := ledger.Replay(accounts, events)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bench:", err)
			os.Exit(1)
		}
		done <- l
	}()
	var l *ledger.Ledger
	select {
	case l = <-done:
	case <-time.After(deadline):
		return result{}, false
	}
	r := result{replay: time.Since(start)}
	stop()

	runtime.GC()
	runtime.ReadMemStats(&after)
	r.heap = int64(after.HeapAlloc) - int64(before.HeapAlloc)

	var report bytes.Buffer
	if withReport {
		start = time.Now()
		l.Report(&report)
		r.report = time.Since(start)
	}

	journal := l.Journal()
	r.lines = len(journal)
	h := sha256.New()
	for _, line := range journal {
		_, _ = fmt.Fprintf(h, "%+v\n", line)
	}
	_, _ = h.Write(report.Bytes())
	copy(r.hash[:], h.Sum(nil))
	return r, true
}

func startProfile(path string) (stop func()) {
	if path == "" {
		return func() {}
	}
	f, err := os.Create(path)
	if err == nil {
		err = pprof.StartCPUProfile(f)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		os.Exit(1)
	}
	return func() {
		pprof.StopCPUProfile()
		_ = f.Close()
	}
}

func median(durations []time.Duration) time.Duration {
	sorted := slices.Clone(durations)
	slices.Sort(sorted)
	return sorted[len(sorted)/2]
}

type generator struct {
	rng      *rand.Rand
	accounts []ledger.Account
	open     map[int][]string // active authorization IDs by account index
}

// generate returns n events spread evenly over the six days in arrival order.
// Even-numbered accounts are AED and odd-numbered accounts BHD, all opening at
// zero.
func generate(n, accountCount int, seed uint64) ([]ledger.Account, []ledger.Event) {
	g := generator{
		rng:  rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
		open: map[int][]string{},
	}
	for i := range accountCount {
		g.accounts = append(g.accounts, ledger.Account{
			ID: fmt.Sprintf("ACC-%06d", i), Opening: ledger.Money{Currency: currencyOf(i)},
		})
	}
	events := make([]ledger.Event, 0, n)
	for i := range n {
		events = append(events, g.next(i, ledger.Day(1+i*6/n)))
	}
	return g.accounts, events
}

// next returns event i, booked on day. Every twentieth event is a debit
// back-valued to a random day up to its booked day. The rest are 35% credits,
// 25% debits, 25% authorizations, and 15% settlements of a random active
// authorization on the same account, or a credit when there is none.
func (g *generator) next(i int, day ledger.Day) ledger.Event {
	a := g.rng.IntN(len(g.accounts))
	e := ledger.Event{
		ID: fmt.Sprintf("E%d", i+1), AccountID: g.accounts[a].ID,
		BookedDay: day, ValueDay: day,
	}
	r := g.rng.IntN(100)
	switch {
	case i%20 == 19:
		e.Kind, e.Amount = ledger.Debit, g.amount(a, 50, 600)
		e.ValueDay = ledger.Day(1 + g.rng.IntN(int(day)))
	case r < 35:
		e.Kind, e.Amount = ledger.Credit, g.amount(a, 50, 1000)
	case r < 60:
		e.Kind, e.Amount = ledger.Debit, g.amount(a, 10, 400)
	case r < 85:
		e.Kind, e.Amount = ledger.Authorization, g.amount(a, 10, 300)
		e.AuthID = fmt.Sprintf("AUTH-%d", i+1)
		g.open[a] = append(g.open[a], e.AuthID)
	case len(g.open[a]) > 0:
		k := g.rng.IntN(len(g.open[a]))
		e.Kind, e.Amount, e.AuthID = ledger.Settlement, g.amount(a, 10, 300), g.open[a][k]
		g.open[a] = slices.Delete(g.open[a], k, k+1)
	default:
		e.Kind, e.Amount = ledger.Credit, g.amount(a, 50, 1000)
	}
	return e
}

// amount is a whole number of currency units between lo and hi.
func (g *generator) amount(account int, lo, hi int64) ledger.Money {
	currency, unit := currencyOf(account), int64(100)
	if currency == ledger.BHD {
		unit = 1000
	}
	return ledger.Money{Currency: currency, Minor: (lo + g.rng.Int64N(hi-lo+1)) * unit}
}

func currencyOf(account int) ledger.Currency {
	if account%2 == 1 {
		return ledger.BHD
	}
	return ledger.AED
}
