package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"whatsapp-bot/config"
	"whatsapp-bot/internal/handlers"
	"whatsapp-bot/internal/services"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	httpSwagger "github.com/swaggo/http-swagger"
)

// @title WhatsApp Bot API
// @version 1.0
// @description A WhatsApp bot API that supports sending messages, images, audio, and documents
// @host localhost:8081
// @BasePath /api/v1
func main() {
	// Disable standard logging
	log.SetOutput(io.Discard)

	// Load config
	cfg := config.NewConfig()

	// Initialize database connection
	db, err := config.ConnectDatabase()
	if err != nil {
		os.Exit(1)
	}
	defer db.Close()

	// Create connection manager
	connectionManager := services.NewConnectionManager(db, cfg)

	// Create HTTP handler
	httpHandler := handlers.NewHTTPHandler(connectionManager)
	router := mux.NewRouter().PathPrefix("/api/v1").Subrouter()

	router.HandleFunc("/send-message", httpHandler.SendMessage).Methods("POST", "OPTIONS")
	router.HandleFunc("/send-image", httpHandler.SendImage).Methods("POST", "OPTIONS")
	router.HandleFunc("/send-audio", httpHandler.SendAudio).Methods("POST", "OPTIONS")
	router.HandleFunc("/send-document", httpHandler.SendDocument).Methods("POST", "OPTIONS")
	router.HandleFunc("/send-typing", httpHandler.SendTyping).Methods("POST", "OPTIONS")
	router.HandleFunc("/upload", httpHandler.HandleUpload).Methods("POST", "OPTIONS")

	// Rotas de autenticação e status
	router.HandleFunc("/qrcode", httpHandler.GetQRCode).Methods("GET", "OPTIONS")
	router.HandleFunc("/qrcode-base64", httpHandler.GetQRCodeBase64).Methods("GET", "OPTIONS")
	router.HandleFunc("/status", httpHandler.GetStatus).Methods("GET", "OPTIONS")

	// Rotas de contatos
	router.HandleFunc("/mark-viewed", httpHandler.MarkContactViewed).Methods("POST", "OPTIONS")
	router.HandleFunc("/check-viewed", httpHandler.CheckContactViewed).Methods("POST", "OPTIONS")

	// Rota WebSocket
	router.HandleFunc("/ws", handlers.WebSocketHandler)

	// Serve os arquivos estáticos do Swagger
	fs := http.FileServer(http.Dir("./docs"))
	router.PathPrefix("/swagger/").Handler(http.StripPrefix("/api/v1/swagger/", fs))

	// Configuração do Swagger UI
	router.PathPrefix("/swagger-ui/").Handler(httpSwagger.Handler(
		httpSwagger.URL("https://unofficial.ligchat.com/api/v1/swagger/swagger.json"),
		httpSwagger.DeepLinking(true),
	))

	mainRouter := mux.NewRouter()
	mainRouter.PathPrefix("/api/v1").Handler(router)

	// Configurar CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})

	// Aplicar middleware CORS
	handler := c.Handler(mainRouter)

	server := &http.Server{
		Addr:    ":8081",
		Handler: handler,
	}

	// Canal para sinais de interrupção
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			os.Exit(1)
		}
	}()

	<-stop

	// Criar contexto com timeout para shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fechar servidor HTTP
	if err := server.Shutdown(ctx); err != nil {
		os.Exit(1)
	}

	// Fechar todas as conexões WhatsApp de forma segura
	if err := connectionManager.CloseAllConnections(); err != nil {
		os.Exit(1)
	}
}
