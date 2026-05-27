package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func GenerateID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func GenerateInvoiceNumber() string {
	now := time.Now()
	return fmt.Sprintf("INV-%d%02d%02d-%04d", now.Year(), now.Month(), now.Day(), time.Now().UnixNano()%10000)
}

func GenerateQuoteNumber() string {
	now := time.Now()
	return fmt.Sprintf("QTE-%d%02d%02d-%04d", now.Year(), now.Month(), now.Day(), time.Now().UnixNano()%10000)
}

func FormatCurrency(amount float64, currency string) string {
	symbol := "¥"
	if currency == "USD" {
		symbol = "$"
	} else if currency == "EUR" {
		symbol = "€"
	}
	return fmt.Sprintf("%s%.2f", symbol, amount)
}

func FormatDuration(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := minutes / 60
	mins := minutes % 60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, mins)
}

func FormatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func FormatDateTime(t time.Time) string {
	return t.Format("2006-01-02 15:04")
}

func ParseDate(dateStr string) string {
	if dateStr == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("2006-01-02")
}

func DaysBetween(start, end time.Time) int {
	return int(end.Sub(start).Hours() / 24)
}

func IsSameDay(t1, t2 time.Time) bool {
	y1, m1, d1 := t1.Date()
	y2, m2, d2 := t2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func StartOfWeek(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return time.Date(t.Year(), t.Month(), t.Day()-weekday+1, 0, 0, 0, 0, t.Location())
}

func StartOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func EndOfMonth(t time.Time) time.Time {
	return StartOfMonth(t).AddDate(0, 1, 0).Add(-time.Second)
}

func StartOfYear(t time.Time) time.Time {
	return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
}
