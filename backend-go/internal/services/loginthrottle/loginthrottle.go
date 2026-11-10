// Package loginthrottle 登录限流/锁定，等价于 TS modules/auth/services/loginThrottler.ts
package loginthrottle

import (
	"time"

	"itops-backend/internal/logger"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
)

const (
	// MaxFailedAttempts 最大失败次数
	MaxFailedAttempts = 5
	// LockoutDuration 锁定时长
	LockoutDuration = 30 * time.Minute
)

// LoginAttemptResult 登录尝试结果
type LoginAttemptResult struct {
	Locked            bool
	LockoutUntil      time.Time
	RemainingAttempts int
}

func parseStoredTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
}

// CheckLoginLockout 检查账户是否处于锁定状态（等价 checkLoginLockout）
func CheckLoginLockout(username string) LoginAttemptResult {
	user, err := repos.Users.GetLockoutStatus(username)
	if err != nil || user == nil {
		return LoginAttemptResult{Locked: false}
	}

	if user.LockedUntil != nil && *user.LockedUntil != "" {
		lockoutUntil, perr := parseStoredTime(*user.LockedUntil)
		if perr == nil && lockoutUntil.After(time.Now()) {
			return LoginAttemptResult{Locked: true, LockoutUntil: lockoutUntil, RemainingAttempts: 0}
		}
		repos.Users.ClearLockout(user.ID)
	}

	remaining := MaxFailedAttempts - user.FailedLoginAttempt
	if remaining < 0 {
		remaining = 0
	}
	return LoginAttemptResult{Locked: false, RemainingAttempts: remaining}
}

// RecordFailedLogin 记录一次失败登录（等价 recordFailedLogin）
func RecordFailedLogin(username string) LoginAttemptResult {
	user, err := repos.Users.GetFailedAttempts(username)
	if err != nil || user == nil {
		return LoginAttemptResult{Locked: false}
	}

	newAttempts := user.FailedLoginAttempt + 1
	now := time.Now()

	if newAttempts >= MaxFailedAttempts {
		lockoutUntil := now.Add(LockoutDuration)
		repos.Users.RecordFailedLoginWithLock(user.ID, newAttempts,
			lockoutUntil.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339))

		logger.Warnf("User %s has been locked out due to too many failed login attempts", username)
		middleware.InvalidateUserCache(user.ID)

		return LoginAttemptResult{Locked: true, LockoutUntil: lockoutUntil, RemainingAttempts: 0}
	}

	repos.Users.RecordFailedLogin(user.ID, newAttempts, now.UTC().Format(time.RFC3339))

	return LoginAttemptResult{Locked: false, RemainingAttempts: MaxFailedAttempts - newAttempts}
}

// ResetFailedLoginAttempts 登录成功后重置失败计数（等价 resetFailedLoginAttempts）
func ResetFailedLoginAttempts(userID string) {
	repos.Users.Unlock(userID)
	middleware.InvalidateUserCache(userID)
}
