package graph

import (
	"github.com/vektah/gqlparser/v2/validator/core"
	"github.com/vektah/gqlparser/v2/validator/rules"
)

// maxErrorsPerRule is the most errors one validation rule reports for a
// document. graphql-js's validate() stops a validation at its maxErrors
// option, 100 by default; here each rule stops at as many, so the answer to a
// document stays small whatever the document: one under the default token
// limit breaks one rule 999,000 times.
const maxErrorsPerRule = 100

// cappedRules are gqlparser's default validation rules, each reporting its
// first maxErrorsPerRule errors of a document, then one saying the rest are
// left out
func cappedRules() *rules.Rules {
	capped := rules.NewDefaultRules()

	for name, rule := range capped.GetInner() {
		capped.ReplaceRule(name, capErrors(name, rule))
	}

	return capped
}

// capErrors wraps rule, named name, to report its first maxErrorsPerRule
// errors, then one saying the rest are left out. The validator runs a rule
// once per document, so each validation counts its own errors.
func capErrors(name string, rule core.RuleFunc) core.RuleFunc {
	return func(observers *core.Events, addError core.AddErrFunc) {
		reported := 0

		rule(observers, func(options ...core.ErrorOption) {
			reported++

			switch {
			case reported <= maxErrorsPerRule:
				addError(options...)
			case reported == maxErrorsPerRule+1:
				addError(core.Message("more than %d %s errors; the rest are left out", maxErrorsPerRule, name))
			}
		})
	}
}
