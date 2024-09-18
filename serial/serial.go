package serial

import (
	"github.com/tarm/serial"
)

// SerialPort представляет последовательный порт.
type SerialPort struct {
	port *serial.Port
	name string // Хранит имя порта
}

// NewSerialPort создает новый экземпляр SerialPort с заданными параметрами.
func NewSerialPort(name string, baud int) (*SerialPort, error) {
	c := &serial.Config{
		Name: name,
		Baud: baud,
	}
	port, err := serial.OpenPort(c)
	if err != nil {
		return nil, err
	}
	return &SerialPort{port: port, name: name}, nil
}

// WriteByte отправляет один байт данных.
func (sp *SerialPort) WriteByte(data byte) error {
	_, err := sp.port.Write([]byte{data})
	return err
}

// ReadByte считывает один байт данных.
func (sp *SerialPort) ReadByte() (byte, error) {
	buf := make([]byte, 1)
	_, err := sp.port.Read(buf)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

// Close закрывает порт.
func (sp *SerialPort) Close() error {
	return sp.port.Close()
}

// SetBaud устанавливает новую скорость передачи.
func (sp *SerialPort) SetBaud(baud int) error {
	// Закрываем текущий порт
	if err := sp.Close(); err != nil {
		return err
	}
	// Открываем новый порт с новой скоростью
	c := &serial.Config{
		Name: sp.name,
		Baud: baud,
	}
	port, err := serial.OpenPort(c)
	if err != nil {
		return err
	}
	sp.port = port
	return nil
}
