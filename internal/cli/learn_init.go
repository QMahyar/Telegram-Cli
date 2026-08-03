// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.

// learn_init.go is the spec-driven init shim for the self-learning loop.
// newLearnConfig builds the per-CLI *entities.Config the cobra teach /
// recall / learnings commands thread through every call site;
// initLearn seeds the entity_lookups table from the canonical-+-aliases
// table declared in spec.Learn.EntityLookupSeeds. Both run once at
// startup; subsequent calls are idempotent (INSERT OR IGNORE on the
// store side, fresh Config build on the registration side).
//
// Soft validation: a runtime regex compile failure (which spec parse
// already rejects at load time) logs to stderr and continues without
// the pattern instead of aborting the CLI. Empty Learn config produces
// a Config with built-in stopwords and a no-op initLearn.

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"sync"

	"telegram-cli/internal/learn/entities"
	"telegram-cli/internal/store"
)

// newLearnConfig returns the per-CLI entity extractor Config the
// teach / recall / normalize call sites pass into the learn package.
// Reads this CLI's spec.Learn.TickerPatterns, Stopwords, and
// Synonyms; patterns that fail to compile at runtime (defense in
// depth — spec parse already validated them) log a warning and are
// skipped. Declared synonyms register on both the returned Config and
// the store package's write-side normalizer.
func newLearnConfig() *entities.Config {
	cfg := entities.NewConfig()
	if re, err := regexp.Compile(`^@[a-zA-Z0-9_]{4,32}$`); err != nil {
		fmt.Fprintf(os.Stderr, "warning: telegram-cli: ticker pattern %q failed to compile: %v\n", `^@[a-zA-Z0-9_]{4,32}$`, err)
	} else {
		cfg.RegisterTickerPattern(re)
	}
	if re, err := regexp.Compile(`^-?100[0-9]{8,14}$`); err != nil {
		fmt.Fprintf(os.Stderr, "warning: telegram-cli: ticker pattern %q failed to compile: %v\n", `^-?100[0-9]{8,14}$`, err)
	} else {
		cfg.RegisterTickerPattern(re)
	}
	if re, err := regexp.Compile(`^[0-9]{5,12}$`); err != nil {
		fmt.Fprintf(os.Stderr, "warning: telegram-cli: ticker pattern %q failed to compile: %v\n", `^[0-9]{5,12}$`, err)
	} else {
		cfg.RegisterTickerPattern(re)
	}
	if re, err := regexp.Compile(`^https://t\.me/[a-zA-Z0-9_]{4,32}(/[0-9]+)?$`); err != nil {
		fmt.Fprintf(os.Stderr, "warning: telegram-cli: ticker pattern %q failed to compile: %v\n", `^https://t\.me/[a-zA-Z0-9_]{4,32}(/[0-9]+)?$`, err)
	} else {
		cfg.RegisterTickerPattern(re)
	}
	cfg.RegisterStopwords(
		"telegram",
		"chat",
	)
	// Spec-declared same-referent synonym folds, registered on BOTH
	// normalizers: the entities.Config drives the read side (recall
	// family keys) and store.RegisterQuerySynonyms the write side
	// (teach query patterns). One map, two registrations — the
	// two-normalizer symmetry the learn loop's key space depends on.
	learnSynonyms := map[string]string{
		"dm":  "direct message",
		"dms": "direct messages",
	}
	cfg.RegisterSynonyms(learnSynonyms)
	store.RegisterQuerySynonyms(learnSynonyms)
	return cfg
}

// initLearn seeds the entity_lookups table from the spec.Learn.
// EntityLookupSeeds declaration. INSERT OR IGNORE on the store side
// makes this idempotent — calling it twice produces the same DB state
// as calling it once. An empty seed map is a no-op (returns nil).
func initLearn(ctx context.Context, db *sql.DB) error {
	_ = ctx
	_ = db
	return nil
}

// learnInitOnce gates runLearnInitOnce so the seed pass happens at
// most once per CLI process. The sync.Once keeps the cost off the
// hot path for repeat command invocations within an MCP session.
var learnInitOnce sync.Once

// runLearnInitOnce opens the canonical store path, fires initLearn
// once per process, and downgrades any failure to a stderr warning.
// Per the self-learning loop's soft-validation contract, a seed
// failure must never abort the CLI — the recall path returns the
// empty envelope if entity_lookups is missing rows, which is the
// same behavior an opt-out CLI sees.
func runLearnInitOnce(ctx context.Context) {
	learnInitOnce.Do(func() {
		dbPath := defaultDBPath("telegram-cli")
		s, err := store.OpenWithContext(ctx, dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: telegram-cli: learn init: open store: %v\n", err)
			return
		}
		defer s.Close()
		if err := initLearn(ctx, s.DB()); err != nil {
			fmt.Fprintf(os.Stderr, "warning: telegram-cli: learn init: %v\n", err)
		}
	})
}
