// Package tx is the transactional install engine (plan D8). Provider
// adapters PLAN — they compose the step vocabulary defined here — and
// this engine EXECUTES: stage → verify → commit, with rollback on any
// failure. Adapter authors write zero transactional code, which is what
// keeps new providers cheap to add and safe by construction.
//
// The Step interface is sealed (unexported apply method) on purpose:
// the step vocabulary lives here so every mutation nexo ever performs
// goes through the same undo bookkeeping.
package tx

import (
	"errors"
	"fmt"

	"github.com/melvicsosa/nexo/internal/ports"
)

// Step is one planned filesystem mutation. Describe is used for
// dry-run output, journaling and error messages.
type Step interface {
	Describe() string
	// apply performs the mutation and returns the undo that reverts
	// it. Sealed: only this package defines steps.
	apply(fsys ports.FS) (undo, error)
}

type undo func(fsys ports.FS) error

// Engine runs plans atomically. Journal is optional (nil disables
// journaling); when set, an interrupted or failed transaction leaves a
// record that `nexo doctor` can surface later.
type Engine struct {
	FS      ports.FS
	Journal Journal
}

// Run applies steps in order. On the first failure it rolls back every
// applied step in reverse and returns an error naming the failed step;
// rollback failures are attached to the same error rather than
// swallowed, because a half-reverted tree is exactly what the user
// needs to hear about.
func (e *Engine) Run(name string, steps []Step) error {
	descs := make([]string, len(steps))
	for i, s := range steps {
		descs[i] = s.Describe()
	}
	var journalID string
	if e.Journal != nil {
		id, err := e.Journal.Begin(name, descs)
		if err != nil {
			return fmt.Errorf("tx %q: journal: %w", name, err)
		}
		journalID = id
	}

	var undos []undo
	for i, s := range steps {
		u, err := s.apply(e.FS)
		if err != nil {
			failure := fmt.Errorf("tx %q: step %d (%s): %w", name, i+1, s.Describe(), err)
			for j := len(undos) - 1; j >= 0; j-- {
				if rbErr := undos[j](e.FS); rbErr != nil {
					failure = errors.Join(failure, fmt.Errorf("rollback: %w", rbErr))
				}
			}
			if e.Journal != nil {
				if jErr := e.Journal.Abort(journalID, failure); jErr != nil {
					failure = errors.Join(failure, fmt.Errorf("journal: %w", jErr))
				}
			}
			return failure
		}
		if u != nil {
			undos = append(undos, u)
		}
	}

	if e.Journal != nil {
		if err := e.Journal.Commit(journalID); err != nil {
			return fmt.Errorf("tx %q: journal commit: %w", name, err)
		}
	}
	return nil
}

// DryRun renders the plan without touching anything.
func DryRun(steps []Step) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.Describe()
	}
	return out
}
