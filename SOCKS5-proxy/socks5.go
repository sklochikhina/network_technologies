package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
)

const (
	SocksVersion = 0x05
	TCP          = 0x01
	IPv4         = 0x01
	DomainName   = 0x03
	Null         = 0x00

	NoAuth = 0x00

	Succeeded               = 0x00
	Failed                  = 0x01
	NotSupportedCommand     = 0x07
	NotSupportedAddressType = 0x08
)

// connectToClient подключение к клиенту
func connectToClient(conn net.Conn) bool {
	buf := make([]byte, 2) // первый байт - версия прокси, второй - количество методов аутентификации
	_, err := io.ReadFull(conn, buf)
	if err != nil {
		log.Printf("Error reading from %s: %v", conn.RemoteAddr().String(), err)
		return true
	}

	if buf[0] != SocksVersion {
		log.Printf("Accepting ONLY SOCKS5 connections, got: %x", buf[0])
		return true
	}

	numMethods := int(buf[1])
	methods := make([]byte, numMethods)
	_, err = io.ReadFull(conn, methods)
	if err != nil {
		log.Printf("Error reading from %s: %v", conn.RemoteAddr().String(), err)
		return true
	}

	_, err = conn.Write([]byte{SocksVersion, NoAuth})
	if err != nil {
		log.Printf("Error writing to %s: %v", conn.RemoteAddr().String(), err)
		return true
	}

	log.Printf("Successful connection with client %s", conn.RemoteAddr().String())
	return false
}

// connectToRemote подключение к удалённому серверу
func connectToRemote(conn net.Conn) net.Conn {
	/*
				+----+-----+-------+------+----------+----------+
		        |VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
		        +----+-----+-------+------+----------+----------+
		        | 1  |  1  | X'00' |  1   | Variable |    2     |
		        +----+-----+-------+------+----------+----------+

		     Where:
		          o  VER    protocol version: X'05'
		          o  REP    Reply field:
		             o  X'00' succeeded
		             o  X'01' general SOCKS server failure
		             o  X'02' connection not allowed by ruleset
		             o  X'03' Network unreachable
		             o  X'04' Host unreachable
		             o  X'05' Connection refused
		             o  X'06' TTL expired
		             o  X'07' Command not supported
		             o  X'08' Address type not supported
		             o  X'09' to X'FF' unassigned
		          o  RSV    RESERVED
		          o  ATYP   address type of following address
	*/

	buf := make([]byte, 4)
	_, err := io.ReadFull(conn, buf)
	if err != nil {
		connectedSend(conn, Failed)
		log.Printf("Error reading from %s: %v", conn.RemoteAddr().String(), err)
		return nil
	}

	// Проверяем версию прокси
	if buf[0] != SocksVersion {
		connectedSend(conn, NotSupportedCommand)
		log.Printf("Accepting ONLY SOCKS5 connections, got: %x", buf[0])
		return nil
	}

	// Проверяем тип соединения
	if buf[1] != TCP {
		connectedSend(conn, NotSupportedCommand)
		log.Printf("Unknown command: %x", buf[1])
		return nil
	}

	var address string

	// Определяем целевой адрес
	switch buf[3] {

	case IPv4:
		tmpAddr := make([]byte, 4)
		_, err := conn.Read(tmpAddr)
		if err != nil {
			connectedSend(conn, Failed)
			log.Printf("Error reading from %s: %v", conn.RemoteAddr().String(), err)
			return nil
		}
		address = net.IP(tmpAddr).String()

	case DomainName:
		lenBuf := make([]byte, 1)
		_, err := conn.Read(lenBuf) // считываем размер доменного имени
		if err != nil {
			connectedSend(conn, Failed)
			log.Printf("Error reading from %s: %v", conn.RemoteAddr().String(), err)
			return nil
		}
		domain := make([]byte, lenBuf[0])
		_, err = io.ReadFull(conn, domain)
		if err != nil {
			connectedSend(conn, Failed)
			log.Printf("Error reading from %s: %v", conn.RemoteAddr().String(), err)
			return nil
		}
		address = string(domain)

	default:
		connectedSend(conn, NotSupportedAddressType)
		log.Printf("Unsupported SOCKS5 address type: %x", buf[3])
		return nil
	}

	portBuf := make([]byte, 2)
	_, err = io.ReadFull(conn, portBuf)
	if err != nil {
		connectedSend(conn, Failed)
		log.Printf("Error reading from %s: %v", conn.RemoteAddr().String(), err)
		return nil
	}

	port := binary.BigEndian.Uint16(portBuf)
	address = fmt.Sprintf("%s:%d", address, port)

	targetConn, err := net.Dial("tcp", address)
	if err != nil {
		log.Printf("Error connecting to %s: %v", address, err)
		connectedSend(conn, Failed)
		return nil
	}

	connectedSend(conn, Succeeded)
	log.Printf("Successfully connected to %s", address)
	return targetConn
}

// connectedSend отправка ответа клиенту
func connectedSend(conn net.Conn, err_code byte) {
	_, err := conn.Write([]byte{SocksVersion, err_code, 0x00, IPv4, Null, Null, Null, Null, Null, Null})
	if err != nil {
		log.Printf("Error writing to %s: %v", conn.RemoteAddr().String(), err)
		return
	}
}

// transferData отправка данных от клиента к удалённому серверу и обратно
func transferData(conn net.Conn, target_conn net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() { // от клиента к серверу
		defer wg.Done()
		defer target_conn.(*net.TCPConn).CloseWrite()

		_, err := io.Copy(target_conn, conn)
		if err != nil {
			log.Printf("Error transferring data from %s: %v", conn.RemoteAddr().String(), err)
		}
	}()

	go func() { // от сервера к клиенту
		defer wg.Done()
		defer conn.(*net.TCPConn).CloseWrite()

		_, err := io.Copy(conn, target_conn)
		if err != nil {
			log.Printf("Error transferring data to %s: %v", conn.RemoteAddr().String(), err)
		}
	}()

	wg.Wait()
}

// handleClient обработка входящего соединения
func handleClient(conn net.Conn) {
	defer conn.Close()

	log.Printf("New connection from %s", conn.RemoteAddr().String())

	if connectToClient(conn) {
		log.Println("Connection to client failed")
		return
	}

	targetConn := connectToRemote(conn)
	if targetConn == nil {
		log.Println("Target connection failed")
		return
	}
	defer targetConn.Close()

	transferData(conn, targetConn)
}

func main() {
	port := flag.String("port", "8080", "Port to listen on")
	flag.Parse()

	parsedPort, err := strconv.Atoi(*port)
	if err != nil || parsedPort <= 0 || parsedPort > 65535 {
		log.Fatalf("Invalid port number: %s", *port)
		return
	}

	listener, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Printf("Error opening port %s: %v", *port, err)
		return
	}
	defer listener.Close()
	log.Printf("Listening on port %s", *port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		go handleClient(conn)
	}
}
