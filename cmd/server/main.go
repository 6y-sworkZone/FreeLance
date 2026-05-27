package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/freelance/workbench/internal/config"
	"github.com/freelance/workbench/internal/db"
	"github.com/freelance/workbench/internal/handlers"
	"github.com/freelance/workbench/internal/middleware"
)

func main() {
	db.Init(config.DBPath)
	defer db.DB.Close()

	os.MkdirAll(config.UploadDir+"/projects", 0755)
	os.MkdirAll(config.UploadDir+"/pdfs", 0755)
	os.MkdirAll(config.UploadDir+"/exports", 0755)

	mux := http.NewServeMux()

	mux.HandleFunc("/login", handlers.ShowLogin)
	mux.HandleFunc("/login/submit", handlers.HandleLogin)
	mux.HandleFunc("/logout", handlers.HandleLogout)

	mux.HandleFunc("/", middleware.Auth(handlers.Home))

	mux.HandleFunc("/clients", middleware.Auth(handlers.ListClients))
	mux.HandleFunc("/clients/new", middleware.Auth(handlers.NewClientForm))
	mux.HandleFunc("/clients/create", middleware.Auth(handlers.CreateClient))
	mux.HandleFunc("/clients/edit", middleware.Auth(handlers.EditClientForm))
	mux.HandleFunc("/clients/update", middleware.Auth(handlers.UpdateClient))
	mux.HandleFunc("/clients/delete", middleware.Auth(handlers.DeleteClient))
	mux.HandleFunc("/clients/detail", middleware.Auth(handlers.ClientDetail))
	mux.HandleFunc("/clients/communication/add", middleware.Auth(handlers.AddCommunication))
	mux.HandleFunc("/clients/communication/delete", middleware.Auth(handlers.DeleteCommunication))
	mux.HandleFunc("/clients/tags", middleware.Auth(handlers.ManageClientTags))
	mux.HandleFunc("/clients/tags/delete", middleware.Auth(handlers.DeleteClientTag))

	mux.HandleFunc("/projects", middleware.Auth(handlers.ListProjects))
	mux.HandleFunc("/projects/new", middleware.Auth(handlers.NewProjectForm))
	mux.HandleFunc("/projects/create", middleware.Auth(handlers.CreateProject))
	mux.HandleFunc("/projects/edit", middleware.Auth(handlers.EditProjectForm))
	mux.HandleFunc("/projects/update", middleware.Auth(handlers.UpdateProject))
	mux.HandleFunc("/projects/delete", middleware.Auth(handlers.DeleteProject))
	mux.HandleFunc("/projects/detail", middleware.Auth(handlers.ProjectDetail))
	mux.HandleFunc("/projects/status", middleware.Auth(handlers.UpdateProjectStatus))
	mux.HandleFunc("/projects/tags", middleware.Auth(handlers.ManageProjectTags))
	mux.HandleFunc("/projects/tags/delete", middleware.Auth(handlers.DeleteProjectTag))

	mux.HandleFunc("/milestones/create", middleware.Auth(handlers.CreateMilestone))
	mux.HandleFunc("/milestones/status", middleware.Auth(handlers.UpdateMilestoneStatus))
	mux.HandleFunc("/milestones/delete", middleware.Auth(handlers.DeleteMilestone))

	mux.HandleFunc("/files/upload", middleware.Auth(handlers.UploadFile))
	mux.HandleFunc("/files/download", middleware.Auth(handlers.DownloadFile))
	mux.HandleFunc("/files/delete", middleware.Auth(handlers.DeleteFile))

	mux.HandleFunc("/time", middleware.Auth(handlers.TimeTracking))
	mux.HandleFunc("/time/start", middleware.Auth(handlers.StartTimer))
	mux.HandleFunc("/time/stop", middleware.Auth(handlers.StopTimer))
	mux.HandleFunc("/time/manual", middleware.Auth(handlers.ManualTimeEntry))
	mux.HandleFunc("/time/delete", middleware.Auth(handlers.DeleteTimeEntry))
	mux.HandleFunc("/time/export", middleware.Auth(handlers.ExportHours))
	mux.HandleFunc("/time/status", middleware.Auth(handlers.GetActiveTimerStatus))
	mux.HandleFunc("/time/summary", middleware.Auth(handlers.TimeSummary))

	mux.HandleFunc("/invoices", middleware.Auth(handlers.ListInvoices))
	mux.HandleFunc("/invoices/new", middleware.Auth(handlers.NewInvoiceForm))
	mux.HandleFunc("/invoices/create", middleware.Auth(handlers.CreateInvoice))
	mux.HandleFunc("/invoices/detail", middleware.Auth(handlers.InvoiceDetail))
	mux.HandleFunc("/invoices/status", middleware.Auth(handlers.UpdateInvoiceStatus))
	mux.HandleFunc("/invoices/delete", middleware.Auth(handlers.DeleteInvoice))
	mux.HandleFunc("/invoices/pdf", middleware.Auth(handlers.GenerateInvoicePDF))

	mux.HandleFunc("/payments/add", middleware.Auth(handlers.AddPayment))
	mux.HandleFunc("/payments/delete", middleware.Auth(handlers.DeletePayment))

	mux.HandleFunc("/quotes", middleware.Auth(handlers.ListQuotes))
	mux.HandleFunc("/quotes/new", middleware.Auth(handlers.NewQuoteForm))
	mux.HandleFunc("/quotes/create", middleware.Auth(handlers.CreateQuote))
	mux.HandleFunc("/quotes/detail", middleware.Auth(handlers.QuoteDetail))
	mux.HandleFunc("/quotes/status", middleware.Auth(handlers.UpdateQuoteStatus))
	mux.HandleFunc("/quotes/delete", middleware.Auth(handlers.DeleteQuote))
	mux.HandleFunc("/quotes/convert", middleware.Auth(handlers.ConvertQuoteToInvoice))
	mux.HandleFunc("/quotes/template/save", middleware.Auth(handlers.SaveQuoteTemplate))
	mux.HandleFunc("/quotes/template/delete", middleware.Auth(handlers.DeleteQuoteTemplate))
	mux.HandleFunc("/quotes/template/get", middleware.Auth(handlers.GetQuoteTemplate))

	mux.HandleFunc("/finance", middleware.Auth(handlers.FinanceDashboard))

	mux.HandleFunc("/profile", middleware.Auth(handlers.ShowProfile))
	mux.HandleFunc("/profile/update", middleware.Auth(handlers.UpdateProfile))
	mux.HandleFunc("/profile/settings", middleware.Auth(handlers.UpdateSettings))
	mux.HandleFunc("/profile/password", middleware.Auth(handlers.ChangePassword))

	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	uploadFs := http.FileServer(http.Dir(config.UploadDir))
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", uploadFs))

	log.Printf("服务器启动: http://localhost:%d", config.Port)
	log.Printf("默认登录: admin / admin123")
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(config.Port), mux))
}
