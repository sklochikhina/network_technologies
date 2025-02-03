package master

import (
	"SnakeGame/connection"
	"SnakeGame/model/common"
	pb "SnakeGame/model/proto"
	"fmt"
	"google.golang.org/protobuf/proto"
	"log"
	"math/rand"
	"net"
	"time"
)

type Master struct {
	Node *common.Node

	announcement *pb.GameAnnouncement
	Players      *pb.GamePlayers
	lastStateMsg int32
}

// NewMaster создает нового мастера
func NewMaster(multicastConn *net.UDPConn, config *pb.GameConfig) *Master {
	unicastConn, err := connection.GetUnicastConn()
	if err != nil {
		log.Fatalf("Error creating unicast socket: %v", err)
	}

	masterIP := unicastConn.LocalAddr().(*net.UDPAddr).IP
	masterPort := unicastConn.LocalAddr().(*net.UDPAddr).Port

	log.Printf("Выделенный локальный адрес мастера: %s:%v\n", masterIP, masterPort)

	masterPlayer := &pb.GamePlayer{
		Name:      proto.String("Master"),          // Имя игрока (для отображения в интерфейсе)
		Id:        proto.Int32(1),                  // Уникальный идентификатор игрока в пределах игры
		Role:      pb.NodeRole_MASTER.Enum(),       // Роль узла в топологии
		Type:      pb.PlayerType_HUMAN.Enum(),      // Тип игрока
		Score:     proto.Int32(0),                  // Число очков, которые набрал игрок
		IpAddress: proto.String(masterIP.String()), // IPv4 или IPv6 адрес игрока в виде строки. Отсутствует в описании игрока-отправителя сообщения
		Port:      proto.Int32(int32(masterPort)),  // Порт UDP-сокета игрока. Отсутствует в описании игрока-отправителя сообщения
	}

	players := &pb.GamePlayers{
		Players: []*pb.GamePlayer{masterPlayer},
	}

	state := &pb.GameState{
		StateOrder: proto.Int32(1),          // Порядковый номер состояния, уникален в пределах игры, монотонно возрастает
		Snakes:     []*pb.GameState_Snake{}, // Список змей
		Foods:      []*pb.GameState_Coord{}, // Список клеток с едой
		Players:    players,                 // Актуальнейший список игроков
	}

	masterSnake := &pb.GameState_Snake{
		PlayerId: proto.Int32(masterPlayer.GetId()), // Идентификатор игрока-владельца змеи, см. GamePlayer.id
		Points: []*pb.GameState_Coord{ // Список "ключевых" точек змеи
			{
				// голова
				X: proto.Int32(config.GetWidth() / 2),
				Y: proto.Int32(config.GetHeight() / 2),
			},
			{
				// хвостик
				X: proto.Int32(config.GetWidth()/2 - 1),
				Y: proto.Int32(config.GetHeight() / 2),
			},
		},
		State:         pb.GameState_Snake_ALIVE.Enum(), // Статус змеи в игре
		HeadDirection: pb.Direction_RIGHT.Enum(),       // Направление, в котором "повёрнута" голова змейки в текущий момент
	}

	// Добавляем мастер-змейку в массив змей
	state.Snakes = append(state.Snakes, masterSnake)

	announcement := &pb.GameAnnouncement{
		Players:  players,                                // Текущие игроки
		Config:   config,                                 // Параметры игры
		CanJoin:  proto.Bool(true),                       // Можно ли новому игроку присоединиться к игре (есть ли место на поле)
		GameName: proto.String(GenerateUniqueGameName()), // Глобально уникальное имя игры, например "my game"
	}

	node := common.NewNode(state, config, multicastConn, unicastConn, masterPlayer)

	return &Master{
		Node:         node,
		announcement: announcement,
		Players:      players,
		lastStateMsg: 0,
	}
}

func GenerateUniqueGameName() string {
	// Инициализация генератора случайных чисел
	rand.Seed(time.Now().UnixNano())

	// Формируем имя из текущей даты/времени и случайного числа
	timestamp := time.Now().Format("20060102_150405") // Формат: ГГГГММДД_ЧЧММСС

	return fmt.Sprintf("game_%s", timestamp)
}

func PromoteToMaster(node *common.Node, state *pb.GameState, config *pb.GameConfig) *Master {
	newMasterPlayer := &pb.GamePlayer{
		Name:      proto.String(node.PlayerInfo.GetName()), // Используем текущее имя заместителя
		Id:        node.PlayerInfo.Id,                      // Не меняем ID игрока
		Role:      pb.NodeRole_MASTER.Enum(),               // Присваиваем роль мастера
		Type:      node.PlayerInfo.Type,                    // Сохраняем тип игрока
		Score:     node.PlayerInfo.Score,                   // Сохраняем текущий счёт
		IpAddress: proto.String(node.PlayerInfo.GetIpAddress()),
		Port:      proto.Int32(node.PlayerInfo.GetPort()),
	}

	// Обновляем список игроков
	state.Players = &pb.GamePlayers{
		Players: updatePlayerRole(state.Players.Players, newMasterPlayer.GetId(), pb.NodeRole_MASTER),
	}

	announcement := &pb.GameAnnouncement{
		Players:  state.Players,
		Config:   config,
		CanJoin:  proto.Bool(true),
		GameName: proto.String(node.GameName),
	}

	newNode := common.NewNode(state, config, node.MulticastConn, node.UnicastConn, newMasterPlayer)

	return &Master{
		Node:         newNode,
		announcement: announcement,
		Players:      state.Players,
		lastStateMsg: 0,
	}
}

// updatePlayerRole обновляет роль игрока в списке
func updatePlayerRole(players []*pb.GamePlayer, id int32, newRole pb.NodeRole) []*pb.GamePlayer {
	for _, player := range players {
		if player.GetId() == id {
			player.Role = newRole.Enum()
		}
	}
	return players
}

// Start запуск мастера
func (m *Master) Start() {
	go m.sendAnnouncementMessage()
	go m.receiveUnicastMessages()
	go m.receiveMulticastMessages()
	go m.checkTimeouts()
	go m.sendStateMessage()
	go m.Node.ResendUnconfirmedMessages(m.Node.Config.GetStateDelayMs())
	go m.Node.SendPings(m.Node.Config.GetStateDelayMs())
}

// SendAnnouncement вызываем один раз при становлении мастером
func (m *Master) SendAnnouncement() {
	announcementMsg := &pb.GameMessage{
		MsgSeq: proto.Int64(1),
		Type: &pb.GameMessage_Announcement{
			Announcement: &pb.GameMessage_AnnouncementMsg{
				Games: []*pb.GameAnnouncement{m.announcement},
			},
		},
	}
	multicastAddr, err := net.ResolveUDPAddr("udp4", m.Node.MulticastAddress)
	if err != nil {
		log.Fatalf("Error resolving multicast address: %v", err)
	}
	m.Node.SendMessage(announcementMsg, multicastAddr)
}

// Многоразовая отправка AnnouncementMsg
func (m *Master) sendAnnouncementMessage() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		announcementMsg := &pb.GameMessage{
			MsgSeq: proto.Int64(1),
			Type: &pb.GameMessage_Announcement{
				Announcement: &pb.GameMessage_AnnouncementMsg{
					Games: []*pb.GameAnnouncement{m.announcement},
				},
			},
		}
		multicastAddr, err := net.ResolveUDPAddr("udp4", m.Node.MulticastAddress)
		if err != nil {
			log.Fatalf("Error resolving multicast address: %v", err)
		}
		m.Node.SendMessage(announcementMsg, multicastAddr)
	}
}

// Получение мультикаст сообщений
func (m *Master) receiveMulticastMessages() {
	for {
		buf := make([]byte, 4096)
		n, addr, err := m.Node.MulticastConn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Error receiving multicast message: %v", err)
			continue
		}

		var msg pb.GameMessage
		err = proto.Unmarshal(buf[:n], &msg)
		if err != nil {
			log.Printf("Error unmarshalling multicast message: %v", err)
			continue
		}

		m.handleMulticastMessage(&msg, addr)
	}
}

// Получение юникаст сообщений
func (m *Master) receiveUnicastMessages() {
	for {
		buf := make([]byte, 4096)
		n, addr, err := m.Node.UnicastConn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Error receiving message: %v", err)
			continue
		}

		var msg pb.GameMessage
		err = proto.Unmarshal(buf[:n], &msg)
		if err != nil {
			log.Printf("Error unmarshalling message: %v", err)
			continue
		}

		m.handleUnicastMessage(&msg, addr)
	}
}

// Рассылаем всем игрокам состояние игры
func (m *Master) sendStateMessage() {
	ticker := time.NewTicker(time.Duration(m.Node.Config.GetStateDelayMs()) * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		m.GenerateFood()
		m.UpdateGameState()

		newStateOrder := m.Node.State.GetStateOrder() + 1
		m.Node.State.StateOrder = proto.Int32(newStateOrder)

		stateMsg := &pb.GameMessage{
			MsgSeq: proto.Int64(m.Node.MsgSeq),
			Type: &pb.GameMessage_State{
				State: &pb.GameMessage_StateMsg{
					State: &pb.GameState{
						StateOrder: proto.Int32(newStateOrder),
						Snakes:     m.Node.State.GetSnakes(),
						Foods:      m.Node.State.GetFoods(),
						Players:    m.Node.State.GetPlayers(),
					},
				},
			},
		}
		allAddrs := m.getAllPlayersUDPAddrs()
		m.sendMessageToAllPlayers(stateMsg, allAddrs)
	}
}

// Получение списка адресов всех игроков (кроме мастера)
func (m *Master) getAllPlayersUDPAddrs() []*net.UDPAddr {
	var addrs []*net.UDPAddr
	for _, player := range m.Players.Players {
		// Исключаем самого мастера из списка получателей и зомби-змеек
		if player.GetId() == m.Node.PlayerInfo.GetId() ||
			common.GetSnakeStateById(player.GetId(), m.Node.State) == pb.GameState_Snake_ZOMBIE {
			continue
		}
		addrStr := fmt.Sprintf("%s:%d", player.GetIpAddress(), player.GetPort())
		addr, err := net.ResolveUDPAddr("udp", addrStr)
		if err != nil {
			log.Printf("Error resolving UDP address for player ID %d: %v", player.GetId(), err)
			continue
		}
		addrs = append(addrs, addr)
	}
	return addrs
}

// Отправка всем игрокам
func (m *Master) sendMessageToAllPlayers(msg *pb.GameMessage, addrs []*net.UDPAddr) {
	for _, addr := range addrs {
		m.Node.SendMessage(msg, addr)
	}
}
