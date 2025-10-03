package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// handleTasks обрабатывает запросы к /tasks
func handleTasks(w http.ResponseWriter, r *http.Request) {
	log.Println("Received POST /tasks")

	// Проверяем что метод POST
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Структура для парсинга JSON запроса
	var req struct {
		URLs []string `json:"urls"`
	}

	// Парсим JSON тело запроса
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	// Проверяем что URLs не пустой
	if len(req.URLs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no urls"})
		return
	}

	// Генерируем уникальный ID для задачи на основе URLs
	id := makeID(req.URLs)

	// Блокируем доступ к tasks для записи
	tasksMu.Lock()

	// Создаем задачу только если она не существует
	if _, exists := tasks[id]; !exists {
		// Создаем новую задачу
		tasks[id] = &Task{
			ID:        id,
			CreatedAt: time.Now().UTC(), // Время в UTC для consistency
			Status:    "pending",        // Начальный статус
		}

		// Добавляем все файлы в задачу
		for _, url := range req.URLs {
			tasks[id].Files = append(tasks[id].Files, File{
				URL:    url,
				Status: "pending", // Каждый файл начинается со статуса pending
			})
		}
	}
	// Разблокируем доступ
	tasksMu.Unlock()

	log.Printf("Task created: %s", id)
	// Возвращаем ID созданной задачи
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
}

// handleTaskByID обрабатывает запросы к /tasks/{id} для получения статуса задачи
func handleTaskByID(w http.ResponseWriter, r *http.Request) {
	// Извлекаем ID из URL пути
	id := strings.TrimPrefix(r.URL.Path, "/tasks/")
	// Исправляем проблему с двойным слешем в URL
	id = strings.ReplaceAll(id, ":/", "://")

	// Проверяем что ID не пустой
	if id == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Блокируем доступ к tasks для чтения
	tasksMu.RLock()
	t, ok := tasks[id] // Ищем задачу по ID
	tasksMu.RUnlock()

	// Если задача не найдена - возвращаем 404
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	// Возвращаем полную информацию о задаче
	writeJSON(w, http.StatusOK, t)
}

// writeJSON вспомогательная функция для отправки JSON ответов
func writeJSON(w http.ResponseWriter, code int, v any) {
	// Устанавливаем заголовок Content-Type
	w.Header().Set("Content-Type", "application/json")
	// Устанавливаем HTTP статус код
	w.WriteHeader(code)
	// Кодируем и отправляем JSON
	_ = json.NewEncoder(w).Encode(v)
}
