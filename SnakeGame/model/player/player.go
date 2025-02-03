package player

import (
	"SnakeGame/connection"
	"SnakeGame/model/common"
	"SnakeGame/model/master"
	pb "SnakeGame/model/proto"
	"fmt"
	"google.golang.org/protobuf/proto"
	"log"
	"net"
	"time"
)

type DiscoveredGame struct {
	Players         *pb.GamePlayers
	Config          *pb.GameConfig
	CanJoin         bool
	GameName        string
	AnnouncementMsg *pb.GameMessage_AnnouncementMsg
	MasterAddr      *net.UDPAddr
}

type Player struct {
	Node *common.Node

	AnnouncementMsg *pb.GameMessage_AnnouncementMsg
	MasterAddr      *net.UDPAddr
	LastStateMsg    int32

	haveId bool

	DiscoveredGames []DiscoveredGame
}

func NewPlayer(multicastConn *net.UDPConn, role pb.NodeRole) *Player {
	unicastConn, err := connection.GetUnicastConn()
	if err != nil {
		log.Fatalf("Error creating unicast socket: %v", err)
	}

	playerIP := unicastConn.LocalAddr().(*net.UDPAddr).IP
	playerPort := unicastConn.LocalAddr().(*net.UDPAddr).Port

	fmt.Printf("Выделенный локальный адрес: %s:%v\n", playerIP, playerPort)

	playerInfo := &pb.GamePlayer{
		Name:      proto.String("Player"),
		Id:        proto.Int32(0),
		Type:      pb.PlayerType_HUMAN.Enum(),
		Role:      role.Enum(),
		Score:     proto.Int32(0),
		IpAddress: proto.String(playerIP.String()),
		Port:      proto.Int32(int32(playerPort)),
	}

	node := common.NewNode(nil, nil, multicastConn, unicastConn, playerInfo)

	return &Player{
		Node:            node,
		AnnouncementMsg: nil,
		MasterAddr:      nil,
		LastStateMsg:    0,

		haveId: false,

		DiscoveredGames: []DiscoveredGame{},
	}
}

func (p *Player) Start() {
	p.Node.StopChan = make(chan struct{})

	// Добавляем горутины в WaitGroup
	p.Node.Wg.Add(5) // Указываем количество горутин (с учётом функции получения мультикаст-сообщений)

	go p.discoverGames()
	go p.receiveUnicastMessages()
	go p.checkTimeouts()
	go p.Node.ResendUnconfirmedMessages(p.Node.Config.GetStateDelayMs())
	go p.Node.SendPings(p.Node.Config.GetStateDelayMs())
}

func (p *Player) Stop() {
	// Создаем канал остановки
	close(p.Node.StopChan)

	// Ждем завершения всех горутин
	/*p.Node.Wg.Wait()
	fmt.Println("Все горутины завершены.")*/
}

func (p *Player) ReceiveMulticastMessages() {
	defer p.Node.Wg.Done()

	for {
		select {
		case <-p.Node.StopChan:
			fmt.Println("Stopping receiveMulticastMessages...")
			return
		default:
			buf := make([]byte, 4096)
			n, addr, err := p.Node.MulticastConn.ReadFromUDP(buf)
			//fmt.Println("ReceiveMulticastMessages ReadFromUDP from ", addr.String())

			if err != nil {
				log.Printf("Error receiving multicast message: %v", err)
				continue
			}

			var msg pb.GameMessage
			err = proto.Unmarshal(buf[:n], &msg)
			if err != nil {
				log.Printf("Error unmarshaling multicast message: %v", err)
				continue
			}

			p.handleMulticastMessage(&msg, addr)
		}
	}
}

func (p *Player) addDiscoveredGame(announcement *pb.GameAnnouncement, addr *net.UDPAddr, announcementMsg *pb.GameMessage_AnnouncementMsg) {
	for index, game := range p.DiscoveredGames {
		if game.GameName == announcement.GetGameName() {
			if game.MasterAddr.String() != addr.String() {
				p.Node.Mu.Lock()
				p.DiscoveredGames[index].MasterAddr = addr
				log.Printf("Existed game '%s' master changed on %s", announcement.GetGameName(), addr.String())
				p.Node.Mu.Unlock()
			}
			return
		}
	}

	newGame := DiscoveredGame{
		Players:         announcement.GetPlayers(),
		Config:          announcement.GetConfig(),
		CanJoin:         announcement.GetCanJoin(),
		GameName:        announcement.GetGameName(),
		AnnouncementMsg: announcementMsg,
		MasterAddr:      addr,
	}

	p.DiscoveredGames = append(p.DiscoveredGames, newGame)
	log.Printf("Discovered new game '%s' with master %s", announcement.GetGameName(), addr.String())
}

func (p *Player) receiveUnicastMessages() {
	defer p.Node.Wg.Done()

	for {
		select {
		case <-p.Node.StopChan:
			fmt.Println("Stopping receiveUnicastMessages...")
			return
		default:
			buf := make([]byte, 4096)
			n, addr, err := p.Node.UnicastConn.ReadFromUDP(buf)
			if err != nil {
				log.Printf("Error receiving message: %v", err)
				continue
			}

			var msg pb.GameMessage
			err = proto.Unmarshal(buf[:n], &msg)
			if err != nil {
				continue
			}

			p.handleUnicastMessage(&msg, addr)
		}
	}
}

// discoverGames ищет доступные игры
func (p *Player) discoverGames() {
	log.Printf("Trying to discover games... %s", p.Node.MulticastAddress)

	discoverMsg := &pb.GameMessage{
		MsgSeq: proto.Int64(p.Node.MsgSeq),
		Type: &pb.GameMessage_Discover{
			Discover: &pb.GameMessage_DiscoverMsg{},
		},
	}

	multicastAddr, err := net.ResolveUDPAddr("udp4", p.Node.MulticastAddress)
	if err != nil {
		log.Fatalf("Error resolving multicast address: %v", err)
		return
	}

	p.Node.SendMessage(discoverMsg, multicastAddr)
	log.Printf("Player: Sent DiscoverMsg to multicast address %v", multicastAddr)
}

func (p *Player) sendJoinRequest() {
	if p.AnnouncementMsg == nil || len(p.AnnouncementMsg.Games) == 0 {
		log.Printf("Player: No available games to join")
		return
	}

	joinMsg := &pb.GameMessage{
		MsgSeq: proto.Int64(p.Node.MsgSeq),
		Type: &pb.GameMessage_Join{
			Join: &pb.GameMessage_JoinMsg{
				PlayerType:    pb.PlayerType_HUMAN.Enum(),
				PlayerName:    p.Node.PlayerInfo.Name,
				GameName:      proto.String(p.AnnouncementMsg.Games[0].GetGameName()),
				RequestedRole: p.Node.Role.Enum(),
			},
		},
	}

	p.Node.SendMessage(joinMsg, p.MasterAddr)
	if p.Node.Role == pb.NodeRole_NORMAL {
		log.Printf("Player: Sent JoinMsg to master at %v", p.MasterAddr)
	} else {
		log.Printf("Viewer: Sent JoinMsg to master at %v", p.MasterAddr)
	}
}

func (p *Player) sendRoleChangeRequest(newRole pb.NodeRole) {
	roleChangeMsg := &pb.GameMessage{
		MsgSeq:   proto.Int64(p.Node.MsgSeq),
		SenderId: proto.Int32(p.Node.PlayerInfo.GetId()),
		Type: &pb.GameMessage_RoleChange{
			RoleChange: &pb.GameMessage_RoleChangeMsg{
				SenderRole:   p.Node.PlayerInfo.GetRole().Enum(),
				ReceiverRole: newRole.Enum(),
			},
		},
	}

	p.Node.SendMessage(roleChangeMsg, p.MasterAddr)
	log.Printf("Player: Sent RoleChangeMsg to %v with new role: %v", p.MasterAddr, newRole)
}

// Обработка отвалившихся узлов
func (p *Player) checkTimeouts() {
	ticker := time.NewTicker(time.Duration(0.8*float64(p.Node.Config.GetStateDelayMs())) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			p.Node.LastInteraction.Range(func(key, value interface{}) bool {
				lastInteraction := value.(time.Time)
				if now.Sub(lastInteraction) > time.Duration(0.8*float64(p.Node.Config.GetStateDelayMs()))*time.Millisecond {
					switch p.Node.PlayerInfo.GetRole() {
					// Игрок заметил, что мастер отвалился и переходит к Deputy
					case pb.NodeRole_NORMAL:
						end := false
						for !end {
							for _, game := range p.DiscoveredGames {
								if game.GameName == p.Node.GameName {
									if p.MasterAddr.String() != game.MasterAddr.String() {
										p.Node.Mu.Lock()
										p.Node.DeleteOldStaff() // удаляем все старые сообщения
										p.MasterAddr = game.MasterAddr
										p.Node.MasterAddr = game.MasterAddr
										p.sendRoleChangeRequest(pb.NodeRole_DEPUTY) // пытается стать заместителем
										log.Printf("Switched to DEPUTY as new MASTER at %v", p.MasterAddr)
										end = true
										p.Node.Mu.Unlock()
									}
									break
								}
							}
						}

					// Deputy заметил, что отвалился мастер и заменяет его
					case pb.NodeRole_DEPUTY:
						p.becomeMaster(false)
					}
				}
				return true
			})
		case <-p.Node.StopChan:
			fmt.Println("Stopping checkTimeouts...")
			return
		}
	}
}

func (p *Player) getDeputy() *pb.GamePlayer {
	for _, player := range p.Node.State.Players.Players {
		if player.GetRole() == pb.NodeRole_MASTER {
			return player
		}
	}
	return nil
}

func (p *Player) becomeMaster(isRemoved bool) {
	// Создаем нового мастера
	masterNode := master.PromoteToMaster(p.Node, p.Node.State, p.Node.Config)
	masterNode.SendAnnouncement()
	//masterNode.Node.MasterAddr =

	addr := fmt.Sprintf("%s:%d", p.Node.PlayerInfo.GetIpAddress(), p.Node.PlayerInfo.GetPort())
	masterNode.Node.MasterAddr, _ = net.ResolveUDPAddr("udp4", addr)

	if !isRemoved {
		for index, player := range p.Node.State.Players.Players {
			for _, snake := range p.Node.State.Snakes {
				// змейку прежнего мастера делаем зомби, если он отвалился
				if player.GetRole() == pb.NodeRole_MASTER {
					//tmp = append(p.Node.State.Players.Players[:index], p.Node.State.Players.Players[index+1:]...)
					if player.GetId() == snake.GetPlayerId() &&
						player.GetId() != p.Node.PlayerInfo.GetId() && snake.State == pb.GameState_Snake_ALIVE.Enum() {
						masterNode.MakeSnakeZombie(player.GetId())
						masterNode.Node.State.Players.Players[index].Role = pb.NodeRole_VIEWER.Enum() // прежнего мастера делаем вьюером
						masterNode.Players.Players = masterNode.Node.State.Players.Players
					}
					continue
				}
			}
		}
	}
	tmp := p.Node.State.Players.Players

	// Удаляем заместителя из списка
	for i, player := range tmp {
		if player.GetId() == p.Node.PlayerInfo.GetId() {
			tmp = append(tmp[:i], tmp[i+1:]...)
		}
	}

	// Добавляем нового мастера в список игроков
	masterNode.Players.Players = append(tmp, masterNode.Node.PlayerInfo)
	masterNode.Node.State.Players = masterNode.Players

	// Останавливаем функции игрока
	log.Printf("DEPUTY becoming new MASTER")
	p.Stop()
	p.Node.NotifyChangeRole(pb.NodeRole_MASTER)
	p.Node.DeleteOldStaff()

	// Запускаем мастера
	go masterNode.Start()
}
