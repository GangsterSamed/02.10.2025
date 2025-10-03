package main

import (
	"encoding/json"
	"os"
)

// loadSnapshot загружает сохраненное состояние задач из файла
func loadSnapshot(path string) error {
	// Читаем файл полностью
	b, err := os.ReadFile(path)
	if err != nil {
		// Если файла нет, особенно при первом запуске
		return nil
	}
	// Структура для парсинга JSON
	var m map[string]*Task
	// Парсим JSON
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}

	// Блокируем доступ к tasks для записи
	tasksMu.Lock()
	defer tasksMu.Unlock() // Гарантируем разблокировку

	// Восстанавливаем все задачи из снапшота
	for k, v := range m {
		// При восстановлении сбрасываем статусы "running" в "pending"
		for i := range v.Files {
			if v.Files[i].Status == "running" {
				v.Files[i].Status = "pending" // Позволяем перезапустить загрузку
			}
		}
		// Также сбрасываем статус всей задачи если она была running
		if v.Status == "running" {
			v.Status = "pending"
		}
		// Сохраняем задачу в глобальное хранилище
		tasks[k] = v
	}
	return nil
}

// saveSnapshot сохраняет текущее состояние всех задач в файл
func saveSnapshot(path string) error {
	// Блокируем доступ к tasks для чтения
	tasksMu.RLock()
	defer tasksMu.RUnlock() // Гарантируем разблокировку

	// Сериализуем задачи в JSON с красивым форматированием
	b, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	// Записываем JSON в файл
	// Права доступа: владелец может читать/писать, остальные только читать
	return os.WriteFile(path, b, 0o644)
}
