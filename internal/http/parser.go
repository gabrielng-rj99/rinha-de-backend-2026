package http

// ParseRequest parses a raw HTTP/1.1 request from buf.
// Returns the method string, path string, body slice (a sub-slice of buf,
// zero-copy), and the total number of bytes consumed (so the caller can
// advance for keep-alive pipelining).
//
// Only POST /fraud-score and GET /ready are expected; the parser is minimal
// and does not allocate for the body.
func ParseRequest(buf []byte) (method, path string, body []byte, bytesRead int) {
	if len(buf) < 4 {
		return "", "", nil, 0
	}

	// ---- method (up to first space) ----
	i := 0
	for i < len(buf) && buf[i] != ' ' {
		i++
	}
	if i >= len(buf) {
		return "", "", nil, 0
	}
	method = string(buf[:i])

	// skip space
	i++

	// ---- path (between first and second space) ----
	pathStart := i
	for i < len(buf) && buf[i] != ' ' {
		i++
	}
	if i >= len(buf) {
		return "", "", nil, 0
	}
	path = string(buf[pathStart:i])

	// skip space and "HTTP/1.1\r\n" (we just scan forward to the first \r\n)
	i++
	for i+1 < len(buf) && !(buf[i] == '\r' && buf[i+1] == '\n') {
		i++
	}
	if i+1 >= len(buf) {
		return "", "", nil, 0
	}
	i += 2 // eat \r\n

	// ---- headers ----
	contentLength := 0

	for i+1 < len(buf) {
		// End of headers => empty header line (just \r\n)
		if buf[i] == '\r' && buf[i+1] == '\n' {
			bodyStart := i + 2
			if contentLength > 0 && bodyStart+contentLength <= len(buf) {
				body = buf[bodyStart : bodyStart+contentLength]
				bytesRead = bodyStart + contentLength
			} else {
				bytesRead = bodyStart
			}
			return
		}

		// Check for "Content-Length:" header (case-sensitive per HTTP/1.1 spec)
		if i+15 < len(buf) &&
			buf[i] == 'C' &&
			buf[i+1] == 'o' &&
			buf[i+2] == 'n' &&
			buf[i+3] == 't' &&
			buf[i+4] == 'e' &&
			buf[i+5] == 'n' &&
			buf[i+6] == 't' &&
			buf[i+7] == '-' &&
			buf[i+8] == 'L' &&
			buf[i+9] == 'e' &&
			buf[i+10] == 'n' &&
			buf[i+11] == 'g' &&
			buf[i+12] == 't' &&
			buf[i+13] == 'h' &&
			buf[i+14] == ':' {
			// Parse the value
			j := i + 15
			for j < len(buf) && buf[j] == ' ' {
				j++
			}
			contentLength = 0
			for j < len(buf) && buf[j] >= '0' && buf[j] <= '9' {
				contentLength = contentLength*10 + int(buf[j]-'0')
				j++
			}
		}

		// Advance to next header line
		for i+1 < len(buf) && !(buf[i] == '\r' && buf[i+1] == '\n') {
			i++
		}
		if i+1 >= len(buf) {
			return "", "", nil, 0
		}
		i += 2 // eat \r\n
	}

	return "", "", nil, 0
}
