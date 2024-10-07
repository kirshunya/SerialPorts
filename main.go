package main

import (
	"bufio"
	"fmt"
	"golang.org/x/sys/windows"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/tarm/serial"
)

const (
	n                   = 1
	flagPrefix          = '$'
	flagSuffix          = 'a' + 1
	dataLength          = n + 1
	escapeChar          = 0x2A
	escapeXOR           = 0x20
	fcsLengthInBits     = 8 // Длина FCS в битах для одиночных ошибок
	fcsLengthInBytes    = (fcsLengthInBits + 7) / 8
	cyclicRedundancyGen = 0x07 // Полином CRC-8
)

type Frame struct {
	Flag               [2]byte
	SourceAddress      byte
	DestinationAddress byte
	Data               [dataLength]byte
	FCS                [fcsLengthInBytes]byte
}

var (
	inputPortName1  string
	outputPortName1 string
	inputPortName2  string
	outputPortName2 string
	baudRate        int
	parity          string
	totalBytesSent1 int
	totalBytesSent2 int
	mu              sync.Mutex
)

func main() {
	rand.Seed(time.Now().UnixNano())

	for {
		selectPortsAndBaudRate()
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

		transmitter2, err := openPort(outputPortName2, baudRate)
		if err != nil {
			log.Fatal(err)
		}
		defer transmitter2.Close()

		receiver2, err := openPort(inputPortName2, baudRate)
		if err != nil {
			log.Fatal(err)
		}
		defer receiver2.Close()

		go receiveData(receiver1, 1)
		go receiveData(receiver2, 2)

		scanner := bufio.NewScanner(os.Stdin)
		fmt.Println("Введите символы для отправки (нажмите Enter для отправки), 'exit' для выхода:")

		for {
			if scanner.Scan() {
				data := scanner.Text()
				if data == "exit" {
					return
				}

				frame1 := createFrame([]byte(data), getPortNumber(outputPortName1))
				encodedFrame1 := byteStuffing(frame1)
				if err := sendData(transmitter1, encodedFrame1); err != nil {
					log.Println("Ошибка отправки на первую пару:", err)
				}
				mu.Lock()
				totalBytesSent1 += len(encodedFrame1)
				mu.Unlock()
				printStatus(1)

				frame2 := createFrame([]byte(data), getPortNumber(outputPortName2))
				encodedFrame2 := byteStuffing(frame2)
				if err := sendData(transmitter2, encodedFrame2); err != nil {
					log.Println("Ошибка отправки на вторую пару:", err)
				}
				mu.Lock()
				totalBytesSent2 += len(encodedFrame2)
				mu.Unlock()
				printStatus(2)
			}
		}

		if err := scanner.Err(); err != nil {
			log.Fatal(err)
		}
	}
}

func sendData(port *serial.Port, data []byte) error {
	_, err := port.Write(data)
	return err
}

func appendWithStuffing(dst []byte, b byte) []byte {
	if b == flagPrefix || b == flagSuffix || b == escapeChar {
		dst = append(dst, escapeChar)
		return append(dst, b^escapeXOR)
	}
	return append(dst, b)
}

func createFrame(data []byte, sourceAddress byte) Frame {
	frame := Frame{
		Flag:               [2]byte{flagPrefix, flagSuffix},
		SourceAddress:      sourceAddress,
		DestinationAddress: 0,
		FCS:                [fcsLengthInBytes]byte{},
	}

	copy(frame.Data[:], data)
	if len(data) < dataLength {
		for i := len(data); i < dataLength; i++ {
			frame.Data[i] = 0
		}
	}

	frame.FCS = calculateFCS(frame.Data[:]) // Вычисление FCS
	return frame
}

func calculateFCS(data []byte) [fcsLengthInBytes]byte {
	var fcs [fcsLengthInBytes]byte
	for _, b := range data {
		for i := 0; i < 8; i++ {
			bit := (b >> (7 - i)) & 0x01
			if (fcs[0]>>7)&0x01 != bit {
				fcs[0] = (fcs[0] << 1) ^ cyclicRedundancyGen
			} else {
				fcs[0] <<= 1
			}
		}
	}
	return fcs
}

func deByteStuffing(frame []byte) []byte {
	var result []byte
	for i := 0; i < len(frame); i++ {
		if frame[i] == escapeChar && i+1 < len(frame) {
			result = append(result, frame[i+1]^escapeXOR)
			i++
		} else {
			result = append(result, frame[i])
		}
	}
	return result
}

func printFrameContent(frame []byte) {
	fmt.Printf("F:'%c%c' ", frame[0], frame[1])
	fmt.Printf("SA:'%d' ", frame[2])
	fmt.Printf("DA:'%c' ", frame[3])
	fmt.Print("Data:'")
	for i := 4; i < len(frame)-fcsLengthInBytes; i++ {
		if frame[i] >= 32 && frame[i] <= 126 {
			fmt.Printf("%c", frame[i])
		} else {
			fmt.Printf("\\x%02X", frame[i])
		}
	}
	fmt.Printf("' FCS:'%x'\n", frame[len(frame)-fcsLengthInBytes]) // Изменено на fcsLengthInBytes
}

func printReceivedFrame(frame []byte, pairNum int) {
	fmt.Printf("Порт %d | Кадр до де-байт-стаффинга: ", pairNum)
	printFrameContent(frame)

	deStuffedFrame := deByteStuffing(frame)
	fmt.Printf("Порт %d | Кадр после де-байт-стаффинга: ", pairNum)

	if len(deStuffedFrame) < 5+fcsLengthInBytes {
		log.Println("Ошибка: недостаточно данных в кадре после де-байт-стаффинга")
		return
	}

	// Подсветка поля FCS
	computedFCS := calculateFCS(deStuffedFrame[4 : 4+dataLength])
	fmt.Print("Data:'")
	for i := 4; i < len(deStuffedFrame)-fcsLengthInBytes; i++ {
		if deStuffedFrame[i] >= 32 && deStuffedFrame[i] <= 126 {
			fmt.Printf("%c", deStuffedFrame[i])
		} else {
			fmt.Printf("\\x%02X", deStuffedFrame[i])
		}
	}
	fmt.Printf("' FCS:'\033[4m%x\033[0m'\n", computedFCS[0]) // Взять только первый элемент для FCS

	// Вставляем случайное искажение одного бита в поле Data
	if rand.Float32() < 0.7 { // Вероятность искажения 70%
		dataIndex := rand.Intn(dataLength)
		if dataIndex < len(deStuffedFrame)-fcsLengthInBytes {
			// Искажение одного бита
			deStuffedFrame[dataIndex+4] ^= 1 << uint(rand.Intn(8))
			fmt.Printf("Случайное искажение бита в Data: %d\n", dataIndex+4)
		}
	}
}

func byteStuffing(frame Frame) []byte {
	var result []byte

	result = append(result, frame.Flag[:]...)
	result = appendWithStuffing(result, frame.SourceAddress)
	result = appendWithStuffing(result, frame.DestinationAddress)
	for _, b := range frame.Data {
		result = appendWithStuffing(result, b)
	}
	for _, b := range frame.FCS {
		result = appendWithStuffing(result, b)
	}

	return result
}

func receiveData(port *serial.Port, pairNum int) {
	buf := make([]byte, 128)
	var frame []byte
	inFrame := false

	for {
		n, err := port.Read(buf)
		if err != nil {
			log.Printf("Ошибка чтения с порта %d: %v\n", pairNum, err)
			continue
		}
		if n > 0 {
			for _, b := range buf[:n] {
				if b == flagPrefix {
					if inFrame {
						printReceivedFrame(frame, pairNum)
					}
					frame = []byte{b}
					inFrame = true
				} else if inFrame {
					frame = append(frame, b)
					if len(frame) >= 5+dataLength+fcsLengthInBytes {
						printReceivedFrame(frame, pairNum)
						inFrame = false
					}
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func printStatus(pairNum int) {
	mu.Lock()
	defer mu.Unlock()
	if pairNum == 1 {
		fmt.Printf("Скорость порта 1: %d, Количество переданных байт: %d\n", baudRate, totalBytesSent1)
	} else {
		fmt.Printf("Скорость порта 2: %d, Количество переданных байт: %d\n", baudRate, totalBytesSent2)
	}
}

func getPortNumber(portName string) byte {
	var num int
	fmt.Sscanf(portName[3:], "%d", &num)
	return byte(num)
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
