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

// RecordFailedLogin 记录一次失败登录（等价 recordFailedLogin）。
// 通过单条原子 UPDATE 递增计数，并发失败不会丢失次数。
func RecordFailedLogin(username string) LoginAttemptResult {
	now := time.Now()
	lockoutUntil := now.Add(LockoutDuration)

	st, err := repos.Users.IncrementFailedLogin(
		username,
		MaxFailedAttempts,
		lockoutUntil.UTC().Format(time.RFC3339),
		now.UTC().Format(time.RFC3339),
	)
	if err != nil || st == nil {
		logger.Errorf("Failed to record failed login for %s: %v", username, err)
		return LoginAttemptResult{Locked: false, RemainingAttempts: MaxFailedAttempts}
	}
	middleware.InvalidateUserCache(st.ID)

	if st.FailedLoginAttempt >= MaxFailedAttempts {
		until := lockoutUntil
		if st.LockedUntil != nil {
			if t, perr := parseStoredTime(*st.LockedUntil); perr == nil {
				until = t
			}
		}
		logger.Warnf("User %s has been locked out due to too many failed login attempts", username)
		return LoginAttemptResult{Locked: true, LockoutUntil: until, RemainingAttempts: 0}
	}

	return LoginAttemptResult{Locked: false, RemainingAttempts: MaxFailedAttempts - st.FailedLoginAttempt}
}

// ResetFailedLoginAttempts 登录成功后重置失败计数（等价 resetFailedLoginAttempts）
func ResetFailedLoginAttempts(userID string) {
	repos.Users.Unlock(userID)
	middleware.InvalidateUserCache(userID)
}
