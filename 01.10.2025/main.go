package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// File представляет информацию о скачиваемом файле
type File struct {
	URL    string `json:"url"`             // URL для скачивания
	Status string `json:"status"`          // Текущий статус: pending, running, done, failed
	Path   string `json:"path,omitempty"`  // Путь к скачанному файлу (если успешно)
	Error  string `json:"error,omitempty"` // Описание ошибки (если failed)
}

// Task представляет задачу на скачивание нескольких файлов
type Task struct {
	ID        string    `json:"id"`         // Уникальный идентификатор задачи
	CreatedAt time.Time `json:"created_at"` // Время создания задачи
	Status    string    `json:"status"`     // Общий статус задачи: pending, running, done
	Files     []File    `json:"files"`      // Список файлов для скачивания
}

// Глобальные переменные для хранения состояния приложения
var (
	tasks    = map[string]*Task{} // Хранилище всех задач
	tasksMu  sync.RWMutex         // Мьютекс для безопасного доступа к tasks из нескольких горутин
	stopping = false              // Флаг остановки приложения
)

func main() {
	// Настройка формата логов
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	// Создание необходимых директорий
	_ = os.MkdirAll("downloads", 0o755) // Для скачанных файлов
	_ = os.MkdirAll("data", 0o755)      // Для файлов состояния
	_ = loadSnapshot("data/state.json") // Загрузка сохраненного состояния при запуске

	// Создание канала для остановки воркера
	stopWorker := make(chan struct{})
	// Запуск фонового воркера для обработки задач
	go worker(stopWorker)

	// Настройка HTTP маршрутов
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks", handleTasks)     // POST - создание задачи
	mux.HandleFunc("/tasks/", handleTaskByID) // GET - получение статуса задачи

	// Создание HTTP сервера
	srv := &http.Server{Addr: ":8080", Handler: mux}

	// Запуск сервера в отдельной горутине
	go func() {
		log.Println("Server starting on :8080")
		// ListenAndServe блокируется, поэтому запускаем в горутине
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Настройка обработки сигналов остановки (Ctrl+C)
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	// Ожидание сигнала остановки
	<-sigc

	log.Println("Shutting down... (5 seconds to accept new tasks)")

	// Процесс graceful shutdown:
	// перестаем начинать новые загрузки
	stopping = true   // Воркер перестанет брать новые задачи
	close(stopWorker) // Сигнал воркеру на остановку

	// Даем 5 секунд на прием НОВЫХ задач
	time.Sleep(5 * time.Second)

	// останавливаем HTTP сервер
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)

	// Сохраняем финальное состояние
	_ = saveSnapshot("data/state.json")

	log.Println("Server stopped")
}
