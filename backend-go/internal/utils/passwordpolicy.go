// Package utils 通用工具，等价于 TS src/utils/
package utils

import (
	"regexp"
	"unicode/utf8"
)

// ───────────────────────────────────────────────
// 密码策略（等价 utils/passwordPolicy.ts）
// ───────────────────────────────────────────────

var (
	reUpper   = regexp.MustCompile(`[A-Z]`)
	reLower   = regexp.MustCompile(`[a-z]`)
	reNumber  = regexp.MustCompile(`[0-9]`)
	reSpecial = regexp.MustCompile(`[!@#$%^&*()\-_=+\[\]{}|;:,.<>?]`)
)

// PasswordDetails 密码复杂度明细
type PasswordDetails struct {
	MinLength bool `json:"minLength"`
	Uppercase bool `json:"uppercase"`
	Lowercase bool `json:"lowercase"`
	Number    bool `json:"number"`
	Special   bool `json:"special"`
}

// PasswordValidationResult 校验结果
type PasswordValidationResult struct {
	Valid   bool             `json:"valid"`
	Message string           `json:"message"`
	Details *PasswordDetails `json:"details,omitempty"`
}

// ValidatePassword 校验密码强度（等价 validatePassword）
func ValidatePassword(password string) PasswordValidationResult {
	details := PasswordDetails{
		MinLength: utf8.RuneCountInString(password) >= 8,
		Uppercase: reUpper.MatchString(password),
		Lowercase: reLower.MatchString(password),
		Number:    reNumber.MatchString(password),
		Special:   reSpecial.MatchString(password),
	}

	if details.MinLength && details.Uppercase && details.Lowercase && details.Number && details.Special {
		return PasswordValidationResult{Valid: true, Message: "密码符合要求", Details: &details}
	}

	var missing []string
	if !details.MinLength {
		missing = append(missing, "至少8位")
	}
	if !details.Uppercase {
		missing = append(missing, "大写字母")
	}
	if !details.Lowercase {
		missing = append(missing, "小写字母")
	}
	if !details.Number {
		missing = append(missing, "数字")
	}
	if !details.Special {
		missing = append(missing, "特殊字符(!@#$%^&*...)")
	}

	msg := "密码复杂度不足，需要包含：" + joinCN(missing)
	return PasswordValidationResult{Valid: false, Message: msg, Details: &details}
}

func joinCN(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "、"
		}
		out += p
	}
	return out
}
