package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/tarm/serial"
	"golang.org/x/sys/windows"
)

var (
	inputPortName1  string
	outputPortName1 string
	inputPortName2  string
	outputPortName2 string
	baudRate        int
	parity          string // Переменная для хранения паритета
)

var (
	totalBytesSent1 int
	totalBytesSent2 int
	mu              sync.Mutex
)

func main() {
	for {
		selectPortsAndBaudRate()
		// Открываем COM-порты для отправки и получения данных
		transmitter1, err := openPort(outputPortName1, baudRate)
		if err != nil {
			log.Fatal(err)
		}
		defer transmitter1.Close()

		receiver1, err := openPort(inputPortName1, baudRate)
		if err != nil {
			log.Fatal(err)
		}
		defer receiver1.Close()

		transmitter2, err := openPort(inputPortName2, baudRate)
		if err != nil {
			log.Fatal(err)
		}
		defer transmitter2.Close()

		receiver2, err := openPort(outputPortName2, baudRate)
		if err != nil {
			log.Fatal(err)
		}
		defer receiver2.Close()

		// Запускаем горутины для получения данных
		go receiveData(receiver1, 1)
		go receiveData(receiver2, 2)

		// Основной цикл для ввода данных
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Println("Введите символы для отправки (нажмите Enter для отправки), 'exit' для выхода:")

		for {
			if scanner.Scan() {
				data := scanner.Text()
				if data == "exit" {
					return // Выход из режима передачи
				}
				if err := sendData(transmitter1, data); err != nil {
					log.Println("Ошибка отправки на первую пару:", err)
				}
				mu.Lock()
				totalBytesSent1 += len(data)
				mu.Unlock()
				printStatus(1)

				if err := sendData(transmitter2, data); err != nil {
					log.Println("Ошибка отправки на вторую пару:", err)
				}
				mu.Lock()
				totalBytesSent2 += len(data)
				mu.Unlock()
				printStatus(2)
			}
		}

		if err := scanner.Err(); err != nil {
			log.Fatal(err)
		}
	}
}

func selectPortsAndBaudRate() {
	pairs, err := getAvailablePortPairs()
	if err != nil {
		log.Fatal(err)
	}

	if len(pairs) < 2 {
		fmt.Println("Недостаточно доступных пар COM-портов.")
		return
	}

	fmt.Println("Доступные пары COM-портов:")
	for i, pair := range pairs {
		fmt.Printf("%d: %s <-> %s\n", i+1, pair[0], pair[1])
	}

	// Ввод выбора первой пары портов
	var choice1 int
	fmt.Print("Выберите номер первой (отправка ->) пары портов (например, 1): ")
	fmt.Scan(&choice1)

	if choice1 < 1 || choice1 > len(pairs) {
		fmt.Println("Неверный выбор. Попробуйте еще раз.")
		return
	}

	outputPortName1 = pairs[choice1-1][0]
	inputPortName1 = pairs[choice1-1][1]

	var choice2 int
	fmt.Print("Выберите номер второй (отправка <-) пары портов (например, 1): ")
	fmt.Scan(&choice2)

	if choice2 < 1 || choice2 > len(pairs) || choice2 == choice1 {
		fmt.Println("Неверный выбор. Попробуйте еще раз.")
		return
	}

	outputPortName2 = pairs[choice2-1][0]
	inputPortName2 = pairs[choice2-1][1]

	fmt.Print("Введите скорость (baud rate) (например, 9600): ")
	fmt.Scan(&baudRate)

	// Добавляем выбор паритета
	fmt.Print("Выберите паритет (None, Even, Odd): ")
	fmt.Scan(&parity)
}

func openPort(portName string, baud int) (*serial.Port, error) {
	c := &serial.Config{Name: portName, Baud: baud}

	// Установка паритета в конфигурации порта
	switch parity {
	case "Even":
		c.Parity = serial.ParityEven
	case "Odd":
		c.Parity = serial.ParityOdd
	default:
		c.Parity = serial.ParityNone
	}

	return serial.OpenPort(c)
}

func sendData(port *serial.Port, data string) error {
	_, err := port.Write([]byte(data))
	return err
}

func receiveData(port *serial.Port, pairNum int) {
	buf := make([]byte, 128)
	for {
		n, err := port.Read(buf)
		if err != nil {
			log.Printf("Ошибка чтения с порта %d: %v\n", pairNum, err)
			continue
		}
		if n > 0 {
			fmt.Printf("Получено на порту %d: %s\n", pairNum, string(buf[:n]))
		}
		time.Sleep(100 * time.Millisecond) // Задержка для предотвращения перегрузки
	}
}

func printStatus(pairNum int) {
	mu.Lock()
	if pairNum == 1 {
		fmt.Printf("Скорость порта 1: %d, Количество переданных байт: %d\n", baudRate, totalBytesSent1)
	} else {
		fmt.Printf("Скорость порта 2: %d, Количество переданных байт: %d\n", baudRate, totalBytesSent2)
	}
	mu.Unlock()
}

func getAvailablePortPairs() ([][]string, error) {
	var pairs [][]string
	var ports []string

	// Получаем список всех доступных портов
	for i := 1; i <= 255; i++ {
		portName := fmt.Sprintf("COM%d", i)
		handle, err := windows.CreateFile(
			windows.StringToUTF16Ptr(portName),
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			0,
			nil,
			windows.OPEN_EXISTING,
			0,
			0,
		)
		if err == nil {
			ports = append(ports, portName)
			windows.CloseHandle(handle)
		}
	}

	portMap := make(map[string]bool)
	for _, port := range ports {
		portMap[port] = true
	}

	for _, port := range ports {
		portNumber := port[3:] // Получаем номер порта
		num := 0
		fmt.Sscanf(portNumber, "%d", &num)

		// Проверяем наличие парного порта
		if num%2 == 1 { // Если нечетный
			pairPort := fmt.Sprintf("COM%d", num+1) // Пара - следующий четный порт
			if portMap[pairPort] {
				pairs = append(pairs, []string{port, pairPort})
			}
		}
	}

	return pairs, nil
}
