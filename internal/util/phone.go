package util

import (
	"regexp"
	"strings"
)

// NormalizePhoneNumber normalizes phone numbers to consistent E.164 format.
// Handles common issues like:
// - Nigerian numbers with extra 0: +23408... → +2348...
// - Other country codes with similar patterns
func NormalizePhoneNumber(phone string) string {
	phone = strings.TrimSpace(phone)

	if phone == "" {
		return phone
	}

	// Nigerian numbers: +234 followed by 0 should drop the 0
	// +23408147993947 → +2348147993947
	if strings.HasPrefix(phone, "+2340") {
		phone = "+234" + phone[5:]
	}

	// Ghana: +233
	if strings.HasPrefix(phone, "+2330") {
		phone = "+233" + phone[5:]
	}

	// Kenya: +254
	if strings.HasPrefix(phone, "+2540") {
		phone = "+254" + phone[5:]
	}

	// South Africa: +27
	if strings.HasPrefix(phone, "+270") {
		phone = "+27" + phone[4:]
	}

	// UK: +44
	if strings.HasPrefix(phone, "+440") {
		phone = "+44" + phone[4:]
	}

	return phone
}

// IsValidE164 checks if phone number is valid E.164 format
func IsValidE164(phone string) bool {
	// E.164: + followed by 1-15 digits
	pattern := `^\+[1-9]\d{6,14}$`
	matched, _ := regexp.MatchString(pattern, phone)
	return matched
}

// NormalizeAndValidate normalizes and validates a phone number
func NormalizeAndValidate(phone string) (string, bool) {
	normalized := NormalizePhoneNumber(phone)
	return normalized, IsValidE164(normalized)
}
