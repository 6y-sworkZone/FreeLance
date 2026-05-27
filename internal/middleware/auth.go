package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/freelance/workbench/internal/db"
	"github.com/freelance/workbench/internal/models"
)

type contextKey string

const UserContextKey contextKey = "user"

func Auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		var session models.Session
		err = db.DB.QueryRow("SELECT id, user_id, expires_at FROM sessions WHERE id = ?", cookie.Value).
			Scan(&session.ID, &session.UserID, &session.ExpiresAt)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		if session.ExpiresAt.Before(time.Now()) {
			db.DB.Exec("DELETE FROM sessions WHERE id = ?", session.ID)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		var user models.User
		err = db.DB.QueryRow("SELECT id, username, name, email, hourly_rate, currency, tax_rate FROM users WHERE id = ?",
			session.UserID).Scan(&user.ID, &user.Username, &user.Name, &user.Email, &user.HourlyRate, &user.Currency, &user.TaxRate)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, &user)
		next(w, r.WithContext(ctx))
	}
}

func GetUser(r *http.Request) *models.User {
	user, ok := r.Context().Value(UserContextKey).(*models.User)
	if !ok {
		return nil
	}
	return user
}
