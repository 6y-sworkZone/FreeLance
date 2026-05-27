package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/freelance/workbench/internal/db"
	"github.com/freelance/workbench/internal/middleware"
	"github.com/freelance/workbench/internal/utils"
	"golang.org/x/crypto/bcrypt"
)

type TemplateData struct {
	Title    string
	User     interface{}
	Data     interface{}
	Flash    string
	Error    string
	Active   string
	CSRF     string
}

func renderTemplate(w http.ResponseWriter, name string, data TemplateData) {
	tmplPath := filepath.Join("templates", name)
	layoutPath := filepath.Join("templates", "layout.html")

	funcMap := template.FuncMap{
		"formatCurrency": func(amount float64, currency string) string {
			return utils.FormatCurrency(amount, currency)
		},
		"formatDuration": func(minutes int) string {
			return utils.FormatDuration(minutes)
		},
		"formatDate": func(t time.Time) string {
			return utils.FormatDate(t)
		},
		"formatDateTime": func(t time.Time) string {
			return utils.FormatDateTime(t)
		},
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"add": func(a, b int) int { return a + b },
		"subtract": func(a, b float64) float64 { return a - b },
		"multiply": func(a, b float64) float64 { return a * b },
		"divideF": func(a, b, defaultVal float64) float64 {
			if b == 0 {
				return defaultVal
			}
			return a / b * defaultVal
		},
		"multiplyF": func(a, b float64) float64 { return a * b },
		"subtractF": func(a, b float64) float64 { return a - b },
		"derefTime": func(t *time.Time) time.Time {
			if t == nil {
				return time.Time{}
			}
			return *t
		},
		"json": func(v interface{}) template.JS {
			b, _ := json.Marshal(v)
			return template.JS(b)
		},
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"contains": strings.Contains,
		"dict": func(values ...interface{}) map[string]interface{} {
			dict := make(map[string]interface{})
			for i := 0; i < len(values); i += 2 {
				dict[values[i].(string)] = values[i+1]
			}
			return dict
		},
	}

	tmpl := template.New("").Funcs(funcMap)
	var err error

	if name == "login.html" {
		tmpl, err = tmpl.ParseFiles(tmplPath)
	} else {
		tmpl, err = tmpl.ParseFiles(layoutPath, tmplPath)
	}

	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if name == "login.html" {
		err = tmpl.ExecuteTemplate(w, name, data)
	} else {
		err = tmpl.ExecuteTemplate(w, "layout", data)
	}

	if err != nil {
		http.Error(w, "Render error: "+err.Error(), http.StatusInternalServerError)
	}
}

func ShowLogin(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "login.html", TemplateData{
		Title: "登录",
	})
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	var userID int64
	var hashedPassword string

	err := db.DB.QueryRow("SELECT id, password FROM users WHERE username = ?", username).
		Scan(&userID, &hashedPassword)
	if err != nil {
		renderTemplate(w, "login.html", TemplateData{
			Title: "登录",
			Error: "用户名或密码错误",
		})
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	if err != nil {
		renderTemplate(w, "login.html", TemplateData{
			Title: "登录",
			Error: "用户名或密码错误",
		})
		return
	}

	sessionID := utils.GenerateID()
	expiresAt := time.Now().Add(24 * time.Hour)

	_, err = db.DB.Exec("INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)",
		sessionID, userID, expiresAt)
	if err != nil {
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionID,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		db.DB.Exec("DELETE FROM sessions WHERE id = ?", cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func ShowProfile(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	renderTemplate(w, "profile.html", TemplateData{
		Title:  "个人设置",
		User:   user,
		Data:   user,
		Active: "profile",
	})
}

func UpdateProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}

	user := middleware.GetUser(r)
	name := r.FormValue("name")
	email := r.FormValue("email")

	_, err := db.DB.Exec("UPDATE users SET name = ?, email = ? WHERE id = ?",
		name, email, user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

func UpdateSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}

	user := middleware.GetUser(r)
	hourlyRate := parseFloat(r.FormValue("hourly_rate"))
	currency := r.FormValue("currency")
	taxRate := parseFloat(r.FormValue("tax_rate"))

	_, err := db.DB.Exec("UPDATE users SET hourly_rate = ?, currency = ?, tax_rate = ? WHERE id = ?",
		hourlyRate, currency, taxRate, user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

func ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}

	user := middleware.GetUser(r)
	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")

	var hashedPassword string
	db.DB.QueryRow("SELECT password FROM users WHERE id = ?", user.ID).Scan(&hashedPassword)

	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(currentPassword)); err != nil {
		http.Error(w, "当前密码错误", http.StatusBadRequest)
		return
	}

	newHashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	db.DB.Exec("UPDATE users SET password = ? WHERE id = ?", string(newHashed), user.ID)
	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}
