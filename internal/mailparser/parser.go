package mailparser

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"time"

	xcharset "golang.org/x/net/html/charset"
)

type Parsed struct {
	From    string
	Subject string
	Date    time.Time
	Text    string
	HTML    string
	Headers map[string][]string
}

var headerDecoder = &mime.WordDecoder{
	CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
		return xcharset.NewReaderLabel(charset, input)
	},
}

func Parse(raw []byte) (Parsed, error) {
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Parsed{}, fmt.Errorf("failed to parse raw message: %w", err)
	}

	out := Parsed{
		From:    decodeHeader(message.Header.Get("From")),
		Subject: decodeHeader(message.Header.Get("Subject")),
		Date:    time.Now().UTC(),
		Headers: cloneHeader(message.Header),
	}

	if date, err := message.Header.Date(); err == nil {
		out.Date = date.UTC()
	}

	text, html, err := parseEntity(textproto.MIMEHeader(message.Header), message.Body)
	if err != nil {
		return Parsed{}, err
	}

	out.Text = strings.TrimSpace(text)
	out.HTML = strings.TrimSpace(html)
	return out, nil
}

func parseEntity(header textproto.MIMEHeader, body io.Reader) (string, string, error) {
	body = decodeTransferEncoding(body, header.Get("Content-Transfer-Encoding"))

	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil || mediaType == "" {
		mediaType = "text/plain"
		params = map[string]string{"charset": "utf-8"}
	}

	switch {
	case strings.HasPrefix(mediaType, "multipart/"):
		boundary := params["boundary"]
		if boundary == "" {
			raw, readErr := io.ReadAll(body)
			if readErr != nil {
				return "", "", fmt.Errorf("failed to read multipart body: %w", readErr)
			}
			return strings.TrimSpace(string(raw)), "", nil
		}

		reader := multipart.NewReader(body, boundary)
		textParts := make([]string, 0, 2)
		htmlParts := make([]string, 0, 2)

		for {
			part, partErr := reader.NextPart()
			if partErr == io.EOF {
				break
			}
			if partErr != nil {
				return strings.Join(textParts, "\n"), strings.Join(htmlParts, "\n"), fmt.Errorf("failed to read multipart part: %w", partErr)
			}

			text, html, entityErr := parseEntity(part.Header, part)
			if entityErr != nil {
				continue
			}
			if strings.TrimSpace(text) != "" {
				textParts = append(textParts, text)
			}
			if strings.TrimSpace(html) != "" {
				htmlParts = append(htmlParts, html)
			}
		}

		return strings.Join(textParts, "\n"), strings.Join(htmlParts, "\n"), nil

	case mediaType == "text/plain":
		content, readErr := readWithCharset(body, params["charset"])
		if readErr != nil {
			return "", "", fmt.Errorf("failed to decode text/plain: %w", readErr)
		}
		return content, "", nil

	case mediaType == "text/html":
		content, readErr := readWithCharset(body, params["charset"])
		if readErr != nil {
			return "", "", fmt.Errorf("failed to decode text/html: %w", readErr)
		}
		return "", content, nil

	default:
		_, _ = io.Copy(io.Discard, body)
		return "", "", nil
	}
}

func readWithCharset(body io.Reader, charsetName string) (string, error) {
	charsetName = strings.ToLower(strings.TrimSpace(charsetName))
	if charsetName != "" && charsetName != "utf-8" && charsetName != "us-ascii" {
		decodedReader, err := xcharset.NewReaderLabel(charsetName, body)
		if err == nil {
			body = decodedReader
		}
	}

	content, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func decodeTransferEncoding(body io.Reader, transferEncoding string) io.Reader {
	switch strings.ToLower(strings.TrimSpace(transferEncoding)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, body)
	case "quoted-printable":
		return quotedprintable.NewReader(body)
	default:
		return body
	}
}

func decodeHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	decoded, err := headerDecoder.DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

func cloneHeader(header mail.Header) map[string][]string {
	out := make(map[string][]string, len(header))
	for key, values := range header {
		out[key] = append([]string(nil), values...)
	}
	return out
}
