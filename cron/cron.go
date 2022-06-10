// Package cron parses cron expressions and computes "fire" dates based on them.
package cron

import (
	"time"
)

// Expr is the representation of a parsed cron expression.
type Expr struct {
	expr string
}

// Parse parses a cron expression and returns, if successful, an Expr object
// that can generate "fire" dates according to the specified expression.
func Parse(expr string) (e Expr, err error) {
	return
}

// MustParse is like Parse, except it panics if the expression is malformed.
func MustParse(expr string) Expr {
	e, err := Parse(expr)
	if err != nil {
		panic(err)
	}
	return e
}

// String returns the string representation of the cron expression.
func (e *Expr) String() string {
	return e.expr
}

// Next computes the next "fire" date after the specified date.
func (e *Expr) Next(from time.Time) time.Time {
	return time.Time{}
}

// Prev computes the previous "fire" date before the specified date.
func (e *Expr) Prev(from time.Time) time.Time {
	return time.Time{}
}
