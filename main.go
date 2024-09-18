package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/tarm/serial"
)

var (
	inputPortName  = "COM3" // Замените на выбранный порт
	outputPortName = "COM4" // Замените на выбранный порт
	baudRate       = 9600   // Фиксированная скорость порта
)

var (
	totalBytesSent int
	mu             sync.Mutex
)

func main() {
	// Открываем COM-порт для отправки данных
	transmitter, err := openPort(outputPortName, baudRate)
	if err != nil {
		log.Fatal(err)
	}
	defer transmitter.Close()

	// Открываем COM-порт для получения данных
	receiver, err := openPort(inputPortName, baudRate)
	if err != nil {
		log.Fatal(err)
	}
	defer receiver.Close()

	// Запускаем горутину для получения данных
	go receiveData(receiver)

	// Основной цикл для ввода данных
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Введите символы для отправки (нажмите Enter для отправки):")

	for scanner.Scan() {
		data := scanner.Text()
		if err := sendData(transmitter, data); err != nil {
			log.Println("Ошибка отправки:", err)
		}
		mu.Lock()
		totalBytesSent += len(data)
		mu.Unlock()
		printStatus()
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}

func openPort(portName string, baud int) (*serial.Port, error) {
	c := &serial.Config{Name: portName, Baud: baud}
	return serial.OpenPort(c)
}

func sendData(port *serial.Port, data string) error {
	_, err := port.Write([]byte(data))
	return err
}

func receiveData(port *serial.Port) {
	buf := make([]byte, 128)
	for {
		n, err := port.Read(buf)
		if err != nil {
			log.Println("Ошибка чтения:", err)
			continue
		}
		if n > 0 {
			fmt.Printf("Получено: %s\n", string(buf[:n]))
		}
		time.Sleep(100 * time.Millisecond) // Задержка для предотвращения перегрузки
	}
}

func printStatus() {
	mu.Lock()
	fmt.Printf("Скорость порта: %d, Количество переданных байт: %d\n", baudRate, totalBytesSent)
	mu.Unlock()
}
