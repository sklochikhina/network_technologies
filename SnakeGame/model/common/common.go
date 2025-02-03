package common

import (
	pb "SnakeGame/model/proto"
	"fmt"
	"google.golang.org/protobuf/proto"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	multicastAddr = "239.192.0.4:9192"
)

// MessageEntry структура для отслеживания неподтверждённых сообщений
type MessageEntry struct {
	msg       *pb.GameMessage
	addr      *net.UDPAddr
	timestamp time.Time
}

// Node общая структура для хранения информации об игроке или мастере
type Node struct {
	GameName         string
	State            *pb.GameState
	Config           *pb.GameConfig
	MulticastAddress string
	MulticastConn    *net.UDPConn
	UnicastConn      *net.UDPConn
	PlayerInfo       *pb.GamePlayer
	MsgSeq           int64
	Role             pb.NodeRole

	MasterAddr *net.UDPAddr

	// Время последнего сообщения от игрока [playerId]time
	LastInteraction sync.Map // map[int32]time.Time

	// Время отправки последнего сообщения другому игроку
	LastSent sync.Map // map[string]time.Time

	unconfirmedMessages sync.Map // map[int64]*MessageEntry
	Mu                  sync.Mutex
	Cond                *sync.Cond
	AckChan             chan int64
	RoleChangeChan      chan pb.NodeRole // Канал для смены роли
	Wg                  sync.WaitGroup
	StopChan            chan struct{}
}

func NewNode(state *pb.GameState, config *pb.GameConfig, multicastConn *net.UDPConn,
	unicastConn *net.UDPConn, playerInfo *pb.GamePlayer) *Node {
	node := &Node{
		State:            state,
		Config:           config,
		MulticastAddress: multicastAddr,
		MulticastConn:    multicastConn,
		UnicastConn:      unicastConn,
		PlayerInfo:       playerInfo,
		Role:             playerInfo.GetRole(),
		MsgSeq:           1,

		LastInteraction:     sync.Map{},
		LastSent:            sync.Map{},
		unconfirmedMessages: sync.Map{},
		AckChan:             make(chan int64),
	}

	node.Cond = sync.NewCond(&node.Mu)

	return node
}

// SendAck любое сообщение подтверждается отправкой в ответ сообщения AckMsg с таким же msg_seq
func (n *Node) SendAck(msg *pb.GameMessage, addr *net.UDPAddr) {
	switch msg.Type.(type) {
	case *pb.GameMessage_Announcement, *pb.GameMessage_Discover, *pb.GameMessage_Ack:
		return
	}

	id := n.GetPlayerIdByAddress(addr)

	ackMsg := &pb.GameMessage{
		MsgSeq:     proto.Int64(msg.GetMsgSeq()),
		SenderId:   proto.Int32(n.PlayerInfo.GetId()),
		ReceiverId: proto.Int32(id),
		Type: &pb.GameMessage_Ack{
			Ack: &pb.GameMessage_AckMsg{},
		},
	}

	n.SendMessage(ackMsg, addr)
}

// GetPlayerIdByAddress id игрока по адресу
func (n *Node) GetPlayerIdByAddress(addr *net.UDPAddr) int32 {
	if n.State == nil {
		return 1
	}
	for _, player := range n.State.Players.Players {
		if player.GetIpAddress() == addr.IP.String() && int(player.GetPort()) == addr.Port {
			return player.GetId()
		}
	}
	return -1
}

// SendPing отправка
func (n *Node) SendPing(addr *net.UDPAddr) {
	pingMsg := &pb.GameMessage{
		MsgSeq:   proto.Int64(n.MsgSeq),
		SenderId: proto.Int32(n.PlayerInfo.GetId()),
		Type: &pb.GameMessage_Ping{
			Ping: &pb.GameMessage_PingMsg{},
		},
	}

	n.SendMessage(pingMsg, addr)
}

// SendMessage отправка сообщения и добавление его в неподтверждённые
func (n *Node) SendMessage(msg *pb.GameMessage, addr *net.UDPAddr) {
	// Увеличиваем порядковый номер сообщения
	msg.SenderId = proto.Int32(n.PlayerInfo.GetId())
	switch msg.Type.(type) {
	case *pb.GameMessage_Ack:
	default:
		msg.MsgSeq = proto.Int64(n.MsgSeq)
		n.MsgSeq++
	}

	// Отправляем
	data, err := proto.Marshal(msg)
	if err != nil {
		log.Printf("Error marshalling Message: %v", err)
		return
	}

	_, err = n.UnicastConn.WriteToUDP(data, addr)
	if err != nil {
		log.Printf("Error sending %s message: %v", GetMsgName(msg), err)
		return
	}

	// Добавляем сообщение в неподтверждённые
	switch msg.Type.(type) {
	case *pb.GameMessage_Announcement, *pb.GameMessage_Discover, *pb.GameMessage_Ack:
	default:
		n.unconfirmedMessages.Store(msg.GetMsgSeq(), &MessageEntry{
			msg:       msg,
			addr:      addr,
			timestamp: time.Now(),
		})
	}

	ip := addr.IP
	port := addr.Port
	address := fmt.Sprintf("%s:%d", ip, port)
	n.LastSent.Store(address, time.Now())
}

// HandleAck обработка полученных AckMsg
func (n *Node) HandleAck(seq int64) {
	if _, exists := n.unconfirmedMessages.Load(seq); exists {
		n.unconfirmedMessages.Delete(seq)
	}
}

// DeleteOldStaff удаляем неподтверждённые прежним мастером сообщения
func (n *Node) DeleteOldStaff() {
	n.unconfirmedMessages.Clear()
	n.LastInteraction.Clear()
	n.LastSent.Clear()
}

// ResendUnconfirmedMessages проверка и переотправка неподтвержденных сообщений
func (n *Node) ResendUnconfirmedMessages(stateDelayMs int32) {
	ticker := time.NewTicker(time.Duration(stateDelayMs/10) * time.Millisecond)
	defer ticker.Stop()
	defer n.Wg.Done()

	for {
		select {
		// Если получили сигнал остановки
		case <-n.StopChan:
			fmt.Println("Stopping ResendUnconfirmedMessages...")
			n.DeleteOldStaff()
			return
		// Ответ не пришёл, заново отправляем сообщение
		case <-ticker.C:
			now := time.Now()
			n.unconfirmedMessages.Range(func(key, value interface{}) bool {
				seq := key.(int64)             // Приведение ключа к типу int64
				entry := value.(*MessageEntry) // Приведение значения к типу *MessageEntry

				if now.Sub(entry.timestamp) > time.Duration(n.Config.GetStateDelayMs()/10)*time.Millisecond {
					// Переотправка сообщения
					data, err := proto.Marshal(entry.msg)
					if err != nil {
						log.Printf("Error marshalling Message: %v", err)
						return true
					}
					_, err = n.UnicastConn.WriteToUDP(data, entry.addr)
					if err != nil {
						fmt.Printf("Error sending Message: %v", err)
						return true
					}

					entry.timestamp = time.Now()
					log.Printf("Resent %s message with Seq: %d to %v from %v", GetMsgName(entry.msg), seq, entry.addr, n.PlayerInfo.GetIpAddress()+":"+strconv.Itoa(int(n.PlayerInfo.GetPort())))
				}
				return true
			})
		// Ответ пришёл, удаляем из мэпы
		case seq := <-n.AckChan:
			n.HandleAck(seq)
		}
	}
}

// SendPings отправка PingMsg, если не было отправлено сообщений в течение stateDelayMs/10
func (n *Node) SendPings(stateDelayMs int32) {
	ticker := time.NewTicker(time.Duration(stateDelayMs/10) * time.Millisecond)
	defer ticker.Stop()
	defer n.Wg.Done()

	for {
		select {
		case <-n.StopChan:
			// Если получили сигнал остановки
			fmt.Println("Stopping SendPings...")
			return
		case <-ticker.C:
			now := time.Now()
			if n.State == nil {
				continue
			}
			if n.Role == pb.NodeRole_MASTER {
				// Мастер пингует всех игроков, кроме себя
				// TODO: починить пингование зомбарей
				for _, player := range n.State.Players.Players {
					if player.GetId() == n.PlayerInfo.GetId() ||
						GetSnakeStateById(player.GetId(), n.State) == pb.GameState_Snake_ZOMBIE {
						continue
					}
					addrKey := fmt.Sprintf("%s:%d", player.GetIpAddress(), player.GetPort())
					lastTime, exists := n.LastSent.Load(addrKey)
					var last time.Time
					if lastTime != nil {
						last = lastTime.(time.Time)
					}
					if !exists || lastTime != nil && now.Sub(last) > time.Duration(n.Config.GetStateDelayMs()/10)*time.Millisecond {
						playerAddr, err := net.ResolveUDPAddr("udp4", addrKey)
						if err != nil {
							log.Printf("Error resolving address for Ping: %v", err)
							continue
						}
						n.SendPing(playerAddr)
						n.LastSent.Store(addrKey, now)
					}
				}
			} else {
				// Обычный игрок пингует только мастера, если мастер известен
				if n.MasterAddr != nil {
					addrKey := n.MasterAddr.String()
					lastTime, exists := n.LastSent.Load(addrKey)
					var last time.Time
					if lastTime != nil {
						last = lastTime.(time.Time)
					}
					if !exists || lastTime != nil && now.Sub(last) > time.Duration(n.Config.GetStateDelayMs()/10)*time.Millisecond {
						n.SendPing(n.MasterAddr)
						n.LastSent.Store(addrKey, now)
					}
				}
			}
		}
	}
}

// NotifyChangeRole посылаем сигнал о смене роли для смены интерфейса
func (n *Node) NotifyChangeRole(newRole pb.NodeRole) {
	if n.Role == newRole {
		return // Роль уже установлена, изменений не требуется
	}

	n.Role = newRole
	log.Printf("Роль изменена на %v", newRole)

	// Отправляем уведомление в канал
	select {
	case n.RoleChangeChan <- newRole:
		log.Printf("Уведомление о смене роли отправлено")
	default:
		log.Printf("Канал RoleChangeChan переполнен, уведомление пропущено")
	}
}

func GetMsgName(msg *pb.GameMessage) string {
	switch msg.Type.(type) {
	case *pb.GameMessage_Discover:
		return "Discover"
	case *pb.GameMessage_Ack:
		return "Ack"
	case *pb.GameMessage_Ping:
		return "Ping"
	case *pb.GameMessage_Announcement:
		return "Announcement"
	case *pb.GameMessage_Steer:
		return "Steer"
	case *pb.GameMessage_Error:
		return "Error"
	case *pb.GameMessage_Join:
		return "Join"
	case *pb.GameMessage_RoleChange:
		return "RoleChange"
	case *pb.GameMessage_State:
		return "State"
	}
	return ""
}

func GetSnakeStateById(id int32, state *pb.GameState) pb.GameState_Snake_SnakeState {
	for _, snake := range state.Snakes {
		if snake.GetPlayerId() == id {
			return snake.GetState()
		}
	}
	return pb.GameState_Snake_ZOMBIE
}
