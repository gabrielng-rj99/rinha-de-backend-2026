package http

// ---------------------------------------------------------------------------
// Hand-rolled JSON field extraction
//
// All functions use anchor-string searching instead of a full JSON parser.
// This means they never build a parse tree – they simply locate the literal
// bytes `"<key>":` in the buffer and then extract the value that follows.
//
// Dotted key paths (e.g. "customer.avg_amount") are handled by first
// narrowing to the parent object, then searching within that sub-object.
//
// Numbers are parsed with hand-written loops – no strconv dependency.
// Strings, bools, null are checked byte-by-byte with no allocations except
// the returned value.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Exported extractors
// ---------------------------------------------------------------------------

// ExtractFloat returns the float64 value at the given dot-separated key path.
// Returns 0 if the key is not found or cannot be parsed.
func ExtractFloat(jsonData []byte, path string) float64 {
	val := locateValue(jsonData, path)
	if val == nil {
		return 0
	}
	return parseFloat64(val)
}

// ExtractInt returns the int value at the given dot-separated key path.
// Returns 0 if the key is not found or cannot be parsed.
func ExtractInt(jsonData []byte, path string) int {
	val := locateValue(jsonData, path)
	if val == nil {
		return 0
	}
	return parseIntFromBytes(val)
}

// ExtractString returns the unquoted string value at the given key path.
// Returns "" if the key is not found.
func ExtractString(jsonData []byte, path string) string {
	val := locateValue(jsonData, path)
	if val == nil {
		return ""
	}
	return unquoteString(val)
}

// ExtractBool returns the bool value at the given key path.
// Returns false if the key is not found or the value is not true/false.
func ExtractBool(jsonData []byte, path string) bool {
	val := locateValue(jsonData, path)
	if val == nil {
		return false
	}
	return parseBool(val)
}

// ExtractStringSlice returns a slice of strings from a JSON array at the
// given key path.
func ExtractStringSlice(jsonData []byte, path string) []string {
	val := locateValue(jsonData, path)
	if val == nil || len(val) == 0 || val[0] != '[' {
		return nil
	}
	return parseStringArray(val)
}

// IsNull returns true when the value at the given key path is the JSON
// literal null.  It returns false if the key is not found or the value is
// anything else (object, string, number, bool, array).
func IsNull(jsonData []byte, path string) bool {
	val := locateValue(jsonData, path)
	if val == nil {
		return false
	}
	return isNullLiteral(val)
}

// ---------------------------------------------------------------------------
// Internal – path navigation & value location
// ---------------------------------------------------------------------------

// locateValue resolves a dot-separated key path and returns the raw bytes
// of the JSON value (including surrounding quotes for strings, braces for
// objects, brackets for arrays, and the literal for primitives).
func locateValue(jsonData []byte, path string) []byte {
	if len(jsonData) == 0 || path == "" {
		return nil
	}

	// Split on the first dot (if any)
	dot := -1
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			dot = i
			break
		}
	}

	if dot < 0 {
		// Leaf key – find it directly
		return extractRawValue(jsonData, path)
	}

	// Navigate into the parent object
	parentKey := path[:dot]
	restKey := path[dot+1:]

	parentVal := extractRawValue(jsonData, parentKey)
	if parentVal == nil || len(parentVal) < 2 || parentVal[0] != '{' {
		return nil
	}

	// Strip the outer braces so the recursive call searches inside
	return locateValue(parentVal[1:len(parentVal)-1], restKey)
}

// ---------------------------------------------------------------------------
// Finding a key and returning its raw value bytes
// ---------------------------------------------------------------------------

// extractRawValue finds "key": in data and returns the raw value bytes
// that follow (quotes for strings, braces for objects, brackets for arrays,
// literal for numbers / booleans / null).
func extractRawValue(data []byte, key string) []byte {
	if len(key) == 0 {
		return nil
	}

	// Search for the literal `"key":` in data
	idx := indexAnchor(data, key)
	if idx < 0 {
		return nil
	}

	// Skip past `"key":`
	start := idx + len(key) + 3 // opening quote + key + `":`
	if start > len(data) {
		return nil
	}

	// Skip whitespace between colon and value
	for start < len(data) && (data[start] == ' ' || data[start] == '\t' ||
		data[start] == '\n' || data[start] == '\r') {
		start++
	}
	if start >= len(data) {
		return nil
	}

	return readValue(data[start:])
}

// indexAnchor locates the literal `"<key>":` inside data without allocating.
// Returns the position of the opening `"` or -1.
func indexAnchor(data []byte, key string) int {
	n := len(data)
	m := len(key)
	if m == 0 || n < m+3 { // need at least "": -> 3 extra chars
		return -1
	}

	// The sequence we look for is `"<key>":`  (3 extra chars: opening ", ":)
	// We iterate byte-by-byte to avoid allocating a []byte from the key.
	for i := 0; i <= n-(m+3); i++ {
		if data[i] != '"' {
			continue
		}
		// Check that the key matches
		j := 0
		for j < m && data[i+1+j] == key[j] {
			j++
		}
		if j < m {
			continue
		}
		// Verify that after the key we have `":`
		if data[i+1+j] == '"' && data[i+2+j] == ':' {
			return i
		}
	}
	return -1
}

// readValue returns the raw JSON value starting at position start.
// start should point to the first character of the value.
func readValue(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}

	switch data[0] {
	case '"':
		// String – find closing quote (with escape support)
		end := 1
		for end < len(data) {
			if data[end] == '\\' {
				end += 2 // skip escaped character
				continue
			}
			if data[end] == '"' {
				end++
				return data[:end]
			}
			end++
		}
		return data // unterminated, return what we have

	case '{':
		// Object – find matching } accounting for nesting
		depth := 1
		end := 1
		for end < len(data) && depth > 0 {
			switch data[end] {
			case '{':
				depth++
			case '}':
				depth--
			case '"':
				// Skip string contents (could contain { } braces)
				end++
				for end < len(data) {
					if data[end] == '\\' {
						end += 2
						continue
					}
					if data[end] == '"' {
						break
					}
					end++
				}
			}
			end++
		}
		return data[:end]

	case '[':
		// Array – find matching ] accounting for nesting
		depth := 1
		end := 1
		for end < len(data) && depth > 0 {
			switch data[end] {
			case '[':
				depth++
			case ']':
				depth--
			case '"':
				end++
				for end < len(data) {
					if data[end] == '\\' {
						end += 2
						continue
					}
					if data[end] == '"' {
						break
					}
					end++
				}
			}
			end++
		}
		return data[:end]

	default:
		// Number, true, false, or null – scan until a delimiter
		end := 0
		for end < len(data) {
			b := data[end]
			if b == ',' || b == '}' || b == ']' || b == ' ' ||
				b == '\t' || b == '\n' || b == '\r' {
				break
			}
			end++
		}
		return data[:end]
	}
}

// ---------------------------------------------------------------------------
// Type-specific value parsers
// ---------------------------------------------------------------------------

// parseFloat64 parses a decimal number from raw bytes.
// Supports optional leading minus and integer + fractional parts.
func parseFloat64(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}

	neg := false
	i := 0
	if data[i] == '-' {
		neg = true
		i++
	}

	// Integer part
	intPart := 0.0
	for i < len(data) && data[i] >= '0' && data[i] <= '9' {
		intPart = intPart*10 + float64(data[i]-'0')
		i++
	}

	// Fractional part
	fracPart := 0.0
	fracDiv := 1.0
	if i < len(data) && data[i] == '.' {
		i++
		for i < len(data) && data[i] >= '0' && data[i] <= '9' {
			fracPart = fracPart*10 + float64(data[i]-'0')
			fracDiv *= 10
			i++
		}
	}

	result := intPart + fracPart/fracDiv
	if neg {
		result = -result
	}
	return result
}

// parseIntFromBytes parses an integer from raw bytes.
func parseIntFromBytes(data []byte) int {
	if len(data) == 0 {
		return 0
	}

	neg := false
	i := 0
	if data[i] == '-' {
		neg = true
		i++
	}

	result := 0
	for i < len(data) && data[i] >= '0' && data[i] <= '9' {
		result = result*10 + int(data[i]-'0')
		i++
	}

	if neg {
		result = -result
	}
	return result
}

// unquoteString removes the surrounding quotes from a JSON string value
// and un-escapes common escape sequences.
func unquoteString(data []byte) string {
	if len(data) < 2 || data[0] != '"' {
		return ""
	}

	// Find the closing quote, handling escapes
	end := 1
	for end < len(data) {
		if data[end] == '\\' {
			end += 2
			continue
		}
		if data[end] == '"' {
			break
		}
		end++
	}
	if end >= len(data) {
		return ""
	}

	// Fast path: no escapes
	hasEscape := false
	for j := 1; j < end; j++ {
		if data[j] == '\\' {
			hasEscape = true
			break
		}
	}

	if !hasEscape {
		return string(data[1:end])
	}

	// Slow path: unescape
	var buf []byte
	for j := 1; j < end; j++ {
		if data[j] == '\\' && j+1 < end {
			j++
			switch data[j] {
			case '"':
				buf = append(buf, '"')
			case '\\':
				buf = append(buf, '\\')
			case '/':
				buf = append(buf, '/')
			case 'b':
				buf = append(buf, '\b')
			case 'f':
				buf = append(buf, '\f')
			case 'n':
				buf = append(buf, '\n')
			case 'r':
				buf = append(buf, '\r')
			case 't':
				buf = append(buf, '\t')
			case 'u':
				if j+4 < end {
					// Simplified unicode escape – just copy the raw
					// sequence for now (not expected in this project)
					buf = append(buf, '\\', 'u')
					for k := 0; k < 4; k++ {
						j++
						buf = append(buf, data[j])
					}
				}
			default:
				buf = append(buf, data[j])
			}
		} else {
			buf = append(buf, data[j])
		}
	}
	return string(buf)
}

// parseBool reads "true" or "false" from the raw value bytes.
func parseBool(data []byte) bool {
	if len(data) >= 4 && data[0] == 't' && data[1] == 'r' &&
		data[2] == 'u' && data[3] == 'e' {
		return true
	}
	return false
}

// isNullLiteral checks if the raw value is the JSON literal null.
func isNullLiteral(data []byte) bool {
	return len(data) >= 4 && data[0] == 'n' && data[1] == 'u' &&
		data[2] == 'l' && data[3] == 'l'
}

// parseStringArray parses a JSON string array like ["a","b","c"].
func parseStringArray(data []byte) []string {
	if len(data) < 2 || data[0] != '[' {
		return nil
	}

	// Remove outer brackets for scanning
	inner := data[1:]
	// Find matching close bracket (handles nesting)
	depth := 1
	closing := 0
	for closing < len(inner) && depth > 0 {
		switch inner[closing] {
		case '[':
			depth++
		case ']':
			depth--
		case '"':
			closing++
			for closing < len(inner) {
				if inner[closing] == '\\' {
					closing += 2
					continue
				}
				if inner[closing] == '"' {
					break
				}
				closing++
			}
		}
		closing++
	}
	inner = inner[:closing-1]

	var result []string
	i := 0
	for i < len(inner) {
		// Skip whitespace
		for i < len(inner) && (inner[i] == ' ' || inner[i] == '\t' ||
			inner[i] == '\n' || inner[i] == '\r') {
			i++
		}
		if i >= len(inner) || inner[i] != '"' {
			break
		}

		// Find end of string
		strStart := i
		i++
		for i < len(inner) {
			if inner[i] == '\\' {
				i += 2
				continue
			}
			if inner[i] == '"' {
				i++
				break
			}
			i++
		}

		s := unquoteString(inner[strStart:i])
		result = append(result, s)

		// Skip comma
		for i < len(inner) && (inner[i] == ',' || inner[i] == ' ' ||
			inner[i] == '\t' || inner[i] == '\n' || inner[i] == '\r') {
			i++
		}
	}

	return result
}
