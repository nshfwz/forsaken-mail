package address

import (
	"fmt"
	"regexp"
	"strings"
)

var mailboxPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._+\-]{0,63}$`)

func ParseEmail(input string) (mailbox string, domain string, err error) {
	value := normalize(input)
	at := strings.LastIndex(value, "@")
	if at <= 0 || at >= len(value)-1 {
		return "", "", fmt.Errorf("invalid email address")
	}

	mailbox = strings.ToLower(strings.TrimSpace(value[:at]))
	domain = strings.ToLower(strings.TrimSpace(value[at+1:]))

	if err := ValidateMailbox(mailbox); err != nil {
		return "", "", err
	}
	if domain == "" {
		return "", "", fmt.Errorf("invalid email domain")
	}

	return mailbox, domain, nil
}

func NormalizeMailbox(input string, expectedDomain string) (mailbox string, email string, err error) {
	value := normalize(input)
	expectedDomain = strings.ToLower(strings.TrimSpace(expectedDomain))

	if strings.Contains(value, "@") {
		localPart, domain, parseErr := ParseEmail(value)
		if parseErr != nil {
			return "", "", parseErr
		}

		if expectedDomain != "" && !strings.EqualFold(domain, expectedDomain) {
			return "", "", fmt.Errorf("email domain must be %s", expectedDomain)
		}

		return localPart, localPart + "@" + domain, nil
	}

	localPart := strings.ToLower(strings.TrimSpace(value))
	if err := ValidateMailbox(localPart); err != nil {
		return "", "", err
	}

	if expectedDomain == "" {
		return localPart, localPart, nil
	}

	return localPart, localPart + "@" + expectedDomain, nil
}

func ValidateMailbox(mailbox string) error {
	if !mailboxPattern.MatchString(mailbox) {
		return fmt.Errorf("invalid mailbox")
	}
	return nil
}

func normalize(input string) string {
	value := strings.TrimSpace(input)
	value = strings.TrimPrefix(value, "<")
	value = strings.TrimSuffix(value, ">")
	return value
}
