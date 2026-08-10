package timeline

import "time"

// int64p 返回 int64 指针。
func int64p(n int64) *int64 { return &n }

// timep 返回 time.Time 指针。
func timep(t time.Time) *time.Time { return &t }
