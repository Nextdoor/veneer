/*
Copyright 2025 Veneer Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package preference

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ParseError represents an error encountered while parsing a preference annotation.
type ParseError struct {
	// AnnotationKey is the full annotation key that failed to parse.
	AnnotationKey string

	// Message describes what went wrong.
	Message string
}

func (e ParseError) Error() string {
	return fmt.Sprintf("annotation %q: %s", e.AnnotationKey, e.Message)
}

// adjustmentRegex matches adjustment expressions like "adjust=+20%", "adjust=-15%", "adjust=5%"
var adjustmentRegex = regexp.MustCompile(`^adjust=([+-]?\d+(?:\.\d+)?)%$`)

// ParseNodePoolPreferences extracts and parses all preference annotations from a NodePool.
//
// Annotation format:
//
//	veneer.io/preference.N: "key=val1,val2 [key2=val3] adjust=[+-]N%"
//
// Where:
//   - N is a positive integer (1-9999) determining the overlay weight/priority
//   - key is a supported Karpenter label key
//   - val1,val2 are comma-separated values to match (In operator)
//   - key!=val uses NotIn operator
//   - key>N uses Gt operator (for numeric labels)
//   - key<N uses Lt operator (for numeric labels)
//   - adjust specifies the price adjustment percentage
//
// Returns the parsed preferences sorted by number (ascending), and any parse errors.
// Parse errors are non-fatal; valid preferences are still returned.
func ParseNodePoolPreferences(annotations map[string]string, nodePoolName string) ([]Preference, []error) {
	preferences := make([]Preference, 0, len(annotations))
	var errors []error

	for key, value := range annotations {
		if !strings.HasPrefix(key, AnnotationPrefix) {
			continue
		}

		// Extract the preference number from the annotation key
		numStr := strings.TrimPrefix(key, AnnotationPrefix)
		num, err := strconv.Atoi(numStr)
		if err != nil {
			errors = append(errors, ParseError{
				AnnotationKey: key,
				Message:       fmt.Sprintf("invalid preference number %q: must be a positive integer", numStr),
			})
			continue
		}
		if num < 1 {
			errors = append(errors, ParseError{
				AnnotationKey: key,
				Message:       fmt.Sprintf("invalid preference number %d: must be >= 1", num),
			})
			continue
		}

		// Parse the preference value
		pref, err := parsePreferenceValue(value)
		if err != nil {
			errors = append(errors, ParseError{
				AnnotationKey: key,
				Message:       err.Error(),
			})
			continue
		}

		pref.Number = num
		pref.NodePoolName = nodePoolName
		preferences = append(preferences, *pref)
	}

	// Sort preferences by number (ascending) for deterministic processing
	sort.Slice(preferences, func(i, j int) bool {
		return preferences[i].Number < preferences[j].Number
	})

	return preferences, errors
}

// parsePreferenceValue parses a single preference annotation value.
//
// Format: "key=val1,val2 [key2=val3] adjust=[+-]N%"
// The "adjust" part is required. All other parts define matchers.
func parsePreferenceValue(value string) (*Preference, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("empty preference value")
	}

	pref := &Preference{}

	// Split on whitespace to get individual expressions
	// We need to be careful to handle the "adjust=..." part specially
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty preference value")
	}

	var adjustFound bool
	for _, part := range parts {
		// Check if this is the adjustment expression
		if strings.HasPrefix(part, "adjust=") {
			adj, err := parseAdjustment(part)
			if err != nil {
				return nil, err
			}
			pref.Adjustment = adj
			adjustFound = true
			continue
		}

		// Otherwise, it's a label matcher
		matcher, err := parseMatcher(part)
		if err != nil {
			return nil, err
		}
		pref.Matchers = append(pref.Matchers, *matcher)
	}

	if !adjustFound {
		return nil, fmt.Errorf("missing required 'adjust=[+-]N%%' expression")
	}

	if len(pref.Matchers) == 0 {
		return nil, fmt.Errorf("at least one label matcher is required")
	}

	return pref, nil
}

// parseMatcher parses a single label matcher expression.
//
// Supported formats:
//   - key=val1,val2  -> In operator
//   - key!=val1,val2 -> NotIn operator
//   - key>N          -> Gt operator (numeric)
//   - key<N          -> Lt operator (numeric)
func parseMatcher(expr string) (*LabelMatcher, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("empty matcher expression")
	}

	matcher := &LabelMatcher{}

	// Try each operator in order of specificity
	// We need to check != before = since = is a substring of !=
	switch {
	case strings.Contains(expr, "!="):
		parts := strings.SplitN(expr, "!=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid matcher %q: expected format key!=val1,val2", expr)
		}
		matcher.Key = strings.TrimSpace(parts[0])
		matcher.Operator = OperatorNotIn
		matcher.Values = parseValues(parts[1])

	case strings.Contains(expr, ">=") || strings.Contains(expr, "<="):
		return nil, fmt.Errorf("invalid matcher %q: >= and <= operators are not supported, use > or <", expr)

	case strings.Contains(expr, ">"):
		parts := strings.SplitN(expr, ">", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid matcher %q: expected format key>N", expr)
		}
		matcher.Key = strings.TrimSpace(parts[0])
		matcher.Operator = OperatorGt
		// Gt operator requires exactly one numeric value
		val := strings.TrimSpace(parts[1])
		if _, err := strconv.Atoi(val); err != nil {
			return nil, fmt.Errorf("invalid matcher %q: Gt operator requires a numeric value, got %q", expr, val)
		}
		matcher.Values = []string{val}

	case strings.Contains(expr, "<"):
		parts := strings.SplitN(expr, "<", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid matcher %q: expected format key<N", expr)
		}
		matcher.Key = strings.TrimSpace(parts[0])
		matcher.Operator = OperatorLt
		// Lt operator requires exactly one numeric value
		val := strings.TrimSpace(parts[1])
		if _, err := strconv.Atoi(val); err != nil {
			return nil, fmt.Errorf("invalid matcher %q: Lt operator requires a numeric value, got %q", expr, val)
		}
		matcher.Values = []string{val}

	case strings.Contains(expr, "="):
		parts := strings.SplitN(expr, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid matcher %q: expected format key=val1,val2", expr)
		}
		matcher.Key = strings.TrimSpace(parts[0])
		matcher.Operator = OperatorIn
		matcher.Values = parseValues(parts[1])

	default:
		return nil, fmt.Errorf("invalid matcher %q: missing operator (=, !=, >, <)", expr)
	}

	// Validate the key is a supported label
	if matcher.Key == "" {
		return nil, fmt.Errorf("invalid matcher %q: empty label key", expr)
	}
	if !SupportedLabels[matcher.Key] {
		return nil, fmt.Errorf("unsupported label key %q: must be one of the supported Karpenter labels", matcher.Key)
	}

	// Validate we have at least one value
	if len(matcher.Values) == 0 {
		return nil, fmt.Errorf("invalid matcher %q: at least one value is required", expr)
	}

	return matcher, nil
}

// parseValues splits a comma-separated value string and trims whitespace.
func parseValues(valuesStr string) []string {
	parts := strings.Split(valuesStr, ",")
	var values []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			values = append(values, p)
		}
	}
	return values
}

// parseAdjustment parses an adjustment expression.
//
// Format: "adjust=[+-]N%" where N is a decimal number.
// Examples: "adjust=-20%", "adjust=+40%", "adjust=15%"
func parseAdjustment(expr string) (float64, error) {
	expr = strings.TrimSpace(expr)

	matches := adjustmentRegex.FindStringSubmatch(expr)
	if matches == nil {
		return 0, fmt.Errorf(
			"invalid adjustment %q: expected format 'adjust=[+-]N%%' (e.g., 'adjust=-20%%')", expr)
	}

	// matches[1] is the captured number (including optional sign)
	adj, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid adjustment %q: %w", expr, err)
	}

	return adj, nil
}
