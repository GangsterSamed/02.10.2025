package main

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// download скачивает файл по URL и сохраняет его на диск
func download(client *http.Client, url string) (string, error) {
	// Выполняем HTTP GET запрос
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	// Гарантируем закрытие тела ответа
	defer resp.Body.Close()
	// Проверяем HTTP статус код (успешные коды 200-299)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", simpleError(resp.Status)
	}

	// Извлекаем имя файла из URL
	baseName := filepath.Base(url)
	// Если имя файла не удалось извлечь, используем дефолтное
	if baseName == "" || baseName == "/" || baseName == "." {
		baseName = "download"
	}

	// Добавляем timestamp к имени файла для гарантии уникальности
	timestamp := time.Now().Format("20060102-150405") // Формат: ГГГГММДД-ЧЧММСС
	uniqueName := timestamp + "-" + baseName
	path := filepath.Join("downloads", uniqueName)

	// Создаем файл для записи
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	// Гарантируем закрытие файла
	defer f.Close()

	// Копируем данные из HTTP ответа в файл
	_, err = io.Copy(f, resp.Body)
	return path, err
}

// simpleError простой тип ошибки для строковых сообщений
type simpleError string

// Error реализует интерфейс error
func (e simpleError) Error() string {
	return string(e)
}

// worker фоновый процессор задач, скачивает файлы по очереди
func worker(stopCh <-chan struct{}) {
	// HTTP клиент с таймаутом 60 секунд
	client := &http.Client{Timeout: 60 * time.Second}

	// Бесконечный цикл обработки
	for {
		// Проверяем флаг остановки приложения
		if stopping {
			log.Println("Worker stopped (shutdown)")
			return
		}

		// Неблокирующая проверка канала остановки
		select {
		case <-stopCh:
			// Получили сигнал остановки по каналу
			log.Println("Worker stopped (channel)")
			return
		default:
			// Продолжаем работу

			// Создаем копию списка задач под блокировкой чтения
			// Это нужно чтобы не держать блокировку долго во время скачивания
			tasksMu.RLock()
			taskList := make([]*Task, 0, len(tasks))
			for _, t := range tasks {
				taskList = append(taskList, t)
			}
			tasksMu.RUnlock()

			// Обрабатываем каждую задачу в списке
			for _, t := range taskList {
				// Обрабатываем каждый файл в задаче
				for i := range t.Files {
					// Пропускаем файлы которые уже обрабатываются или завершены
					if t.Files[i].Status != "pending" {
						continue
					}

					// Помечаем файл как "в процессе скачивания"
					tasksMu.Lock()
					t.Files[i].Status = "running"
					// Если вся задача была в pending, помечаем ее как running
					if t.Status == "pending" {
						t.Status = "running"
					}
					tasksMu.Unlock()

					// Скачиваем файл (это может занять много времени)
					path, err := download(client, t.Files[i].URL)

					// Обновляем статус файла после скачивания
					tasksMu.Lock()
					if err != nil {
						// Если ошибка - помечаем как failed и сохраняем ошибку
						t.Files[i].Status = "failed"
						t.Files[i].Error = err.Error()
					} else {
						// Если успешно - помечаем как done и сохраняем путь
						t.Files[i].Status = "done"
						t.Files[i].Path = path
					}

					// Проверяем завершена ли вся задача
					// Задача считается завершенной когда ВСЕ файлы имеют статус "done"
					allDone := true
					for _, f := range t.Files {
						if f.Status != "done" {
							allDone = false
							break
						}
					}
					if allDone {
						t.Status = "done"
					}
					tasksMu.Unlock()

					// Сохраняем состояние после обработки каждого файла
					// Это обеспечивает persistence при внезапных падениях
					_ = saveSnapshot("data/state.json")
				}
			}
		}
	}
}

// makeID генерирует уникальный идентификатор задачи на основе списка URL
func makeID(urls []string) string {
	// Используем SHA1 хэш для создания уникального ID
	h := sha1.New()
	// Добавляем все URL в хэш
	for _, u := range urls {
		h.Write([]byte(u))
	}
	// Возвращаем hex-представление хэша
	return hex.EncodeToString(h.Sum(nil))
}
