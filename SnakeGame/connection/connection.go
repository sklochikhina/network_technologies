package connection

import (
	"fmt"
	"golang.org/x/net/ipv4"
	"log"
	"net"
)

const (
	multicastAddress = "239.192.0.4:9192"
)

func Connection() *net.UDPConn {
	// Резолвим multicast-адрес
	multicastUDPAddr, err := net.ResolveUDPAddr("udp4", multicastAddress) // udp4 = UDP + IpV4
	if err != nil {
		log.Fatalf("Error resolving multicast address: %v", err)
	}
	multicastConn, err := net.ListenUDP("udp4", multicastUDPAddr)
	if err != nil {
		log.Fatalf("Error creating multicast socket: %v", err)
	}

	iface, err := net.InterfaceByName("Беспроводная сеть")
	if err != nil {
		log.Fatalf("Error getting interface: %v", err)
	}

	pc := ipv4.NewPacketConn(multicastConn)

	// Присоединяемся к multicast-группе
	if err := pc.JoinGroup(iface, multicastUDPAddr); err != nil {
		log.Fatalf("Join group error: %v", err)
	}

	// Проверяем и включаем MulticastLoopback
	if loop, err := pc.MulticastLoopback(); err == nil {
		fmt.Printf("MulticastLoopback status: %v\n", loop)
		if !loop {
			if err := pc.SetMulticastLoopback(true); err != nil {
				fmt.Printf("SetMulticastLoopback error: %v\n", err)
			}
		}
	}

	return multicastConn
}

// GetUnicastConn получение udp-connection
func GetUnicastConn() (*net.UDPConn, error) {
	iface, err := net.InterfaceByName("Беспроводная сеть")
	if err != nil {
		log.Fatalf("Error getting interface: %v", err)
	}

	// Получаем первый IPv4-адрес интерфейса
	addrs, err := iface.Addrs()
	if err != nil {
		log.Fatalf("Error getting addresses: %v", err)
	}

	var localAddr *net.UDPAddr
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && ipNet.IP.To4() != nil {
			localAddr = &net.UDPAddr{
				IP:   ipNet.IP,
				Port: 0, // Автоматический выбор порта
			}
			break
		}
	}

	if localAddr != nil {
		// Создаем сокет для отправки сообщений
		//unicastConn, err := net.DialUDP("udp4", nil, multicastAddr)
		unicastConn, err := net.ListenUDP("udp4", nil)
		if err != nil {
			return nil, fmt.Errorf("error connecting to Multicast: %v", err)
		}
		return unicastConn, err
	}

	return nil, fmt.Errorf("no IPv4 address found for interface %s", iface)
}
