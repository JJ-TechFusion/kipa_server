package util

import (
	"regexp"
	"strings"
)
func NormalizePhoneNumber(phone string) string {
	phone = strings.TrimSpace(phone)

	if phone == "" {
		return phone
	}
	if strings.HasPrefix(phone, "+2340") {
		phone = "+234" + phone[5:]
	}
	if strings.HasPrefix(phone, "+2330") {
		phone = "+233" + phone[5:]
	}
	if strings.HasPrefix(phone, "+2540") {
		phone = "+254" + phone[5:]
	}
	if strings.HasPrefix(phone, "+270") {
		phone = "+27" + phone[4:]
	}
	if strings.HasPrefix(phone, "+440") {
		phone = "+44" + phone[4:]
	}

	return phone
}
func IsValidE164(phone string) bool {
	pattern := `^\+[1-9]\d{6,14}$`
	matched, _ := regexp.MatchString(pattern, phone)
	return matched
}
func NormalizeAndValidate(phone string) (string, bool) {
	normalized := NormalizePhoneNumber(phone)
	return normalized, IsValidE164(normalized)
}
