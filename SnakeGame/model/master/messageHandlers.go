package master

import (
	pb "SnakeGame/model/proto"
	"fmt"
	"google.golang.org/protobuf/proto"
	"log"
	"net"
	"os"
	"time"
)

// Обработка мультикаст сообщений
func (m *Master) handleMulticastMessage(msg *pb.GameMessage, addr *net.UDPAddr) {
	switch /*t :=*/ msg.Type.(type) {
	case *pb.GameMessage_Discover:
		// Отправляем AnnouncementMsg в ответ на DiscoverMsg
		announcementMsg := &pb.GameMessage{
			MsgSeq: proto.Int64(m.Node.MsgSeq),
			Type: &pb.GameMessage_Announcement{
				Announcement: &pb.GameMessage_AnnouncementMsg{
					Games: []*pb.GameAnnouncement{m.announcement},
				},
			},
		}
		m.Node.SendMessage(announcementMsg, addr)
	default:
	}
}

// Обработка юникаст сообщения
func (m *Master) handleUnicastMessage(msg *pb.GameMessage, addr *net.UDPAddr) {
	if msg.GetSenderId() > 0 {
		m.Node.LastInteraction.Store(msg.GetSenderId(), time.Now())
	}
	// Любое сообщение (кроме AnnouncementMsg, DiscoverMsg и AckMsg) подтверждается путём отправки в ответ
	// сообщения AckMsg с таким же msg_seq, как в исходном сообщении.
	switch t := msg.Type.(type) {
	case *pb.GameMessage_Join:
		if t.Join.GetRequestedRole() == pb.NodeRole_VIEWER {
			// Обрабатываем joinMsg для вьюера
			joinMsg := t.Join
			m.handleJoinMessageViewer(msg.GetMsgSeq(), joinMsg, addr)
			break
		}
		// Проверяем, есть ли место 5*5 для новой змейки
		hasSquare, coord := m.hasFreeSquare(m.Node.State, m.Node.Config, 5)

		if !hasSquare {
			m.announcement.CanJoin = proto.Bool(false)
			m.sendErrorMsg(addr)
			log.Printf("Player cannot join: no available space")
			m.Node.SendAck(msg, addr)
		} else {
			// Обрабатываем joinMsg для игрока
			joinMsg := t.Join
			m.handleJoinMessage(msg.GetMsgSeq(), joinMsg, addr, coord)
		}

	case *pb.GameMessage_Discover:
		m.handleDiscoverMessage(addr)

	case *pb.GameMessage_Steer:
		playerId := msg.GetSenderId()
		if playerId == 0 {
			playerId = m.Node.GetPlayerIdByAddress(addr)
		}
		if playerId != 0 {
			m.handleSteerMessage(t.Steer, playerId)
			m.Node.SendAck(msg, addr)
		} else {
			log.Printf("SteerMsg received from unknown address: %v", addr)
		}

	case *pb.GameMessage_RoleChange:
		m.handleRoleChangeMessage(msg, addr)
		m.Node.SendAck(msg, addr)

	case *pb.GameMessage_Ping:
		m.Node.SendAck(msg, addr)

	case *pb.GameMessage_Ack:
		m.Node.AckChan <- msg.GetMsgSeq()

	case *pb.GameMessage_State:
		if t.State.GetState().GetStateOrder() <= m.lastStateMsg {
			return
		} else {
			m.lastStateMsg = t.State.GetState().GetStateOrder()
		}
		m.Node.SendAck(msg, addr)

	default:
		log.Printf("Received unknown message type from %v", addr)
	}
}

func (m *Master) sendErrorMsg(addr *net.UDPAddr) {
	errorMsg := &pb.GameMessage{
		Type: &pb.GameMessage_Error{
			Error: &pb.GameMessage_ErrorMsg{
				ErrorMessage: proto.String("Cannot join: no available space"),
			},
		},
	}
	m.Node.SendMessage(errorMsg, addr)
}

func (m *Master) findIdInPlayers(id int32) bool {
	for _, player := range m.Players.Players {
		if id == player.GetId() {
			return true
		}
	}
	return false
}

func (m *Master) handleJoinMessage(msgSeq int64, joinMsg *pb.GameMessage_JoinMsg, addr *net.UDPAddr, coord *pb.GameState_Coord) {
	newPlayerID := int32(len(m.Players.Players) + 1)
	for m.findIdInPlayers(newPlayerID) { // если такой id уже есть у кого-либо из игроков, то увеличиваем
		newPlayerID += 1
	}

	newPlayer := &pb.GamePlayer{
		Name:      proto.String(joinMsg.GetPlayerName()),
		Id:        proto.Int32(newPlayerID),
		IpAddress: proto.String(addr.IP.String()),
		Port:      proto.Int32(int32(addr.Port)),
		Role:      joinMsg.GetRequestedRole().Enum(),
		Type:      joinMsg.GetPlayerType().Enum(),
		Score:     proto.Int32(0),
	}
	m.Players.Players = append(m.Players.Players, newPlayer)
	m.Node.State.Players = m.Players

	ackMsg := &pb.GameMessage{
		MsgSeq:     proto.Int64(msgSeq),
		SenderId:   proto.Int32(m.Node.PlayerInfo.GetId()),
		ReceiverId: proto.Int32(newPlayerID),
		Type: &pb.GameMessage_Ack{
			Ack: &pb.GameMessage_AckMsg{},
		},
	}
	m.Node.SendMessage(ackMsg, addr)
	m.addSnakeForNewPlayer(newPlayerID, coord)
	m.checkAndAssignDeputy()

	log.Printf("New player joined, ID: %v", newPlayer)
}

func (m *Master) handleJoinMessageViewer(msgSeq int64, joinMsg *pb.GameMessage_JoinMsg, addr *net.UDPAddr) {
	newViewerID := int32(len(m.Players.Players) + 1)
	for m.findIdInPlayers(newViewerID) { // если такой id уже есть у кого-либо из игроков, то увеличиваем
		newViewerID += 1
	}

	newViewer := &pb.GamePlayer{
		Name:      proto.String(joinMsg.GetPlayerName()),
		Id:        proto.Int32(newViewerID),
		IpAddress: proto.String(addr.IP.String()),
		Port:      proto.Int32(int32(addr.Port)),
		Role:      joinMsg.GetRequestedRole().Enum(),
		Type:      joinMsg.GetPlayerType().Enum(),
		Score:     proto.Int32(0),
	}
	m.Players.Players = append(m.Players.Players, newViewer)
	m.Node.State.Players = m.Players

	ackMsg := &pb.GameMessage{
		MsgSeq:     proto.Int64(msgSeq),
		SenderId:   proto.Int32(m.Node.PlayerInfo.GetId()),
		ReceiverId: proto.Int32(newViewerID),
		Type: &pb.GameMessage_Ack{
			Ack: &pb.GameMessage_AckMsg{},
		},
	}
	m.Node.SendMessage(ackMsg, addr)

	log.Printf("New viewer joined, ID: %v", newViewer)
}

// Назначение заместителя
func (m *Master) checkAndAssignDeputy() {
	if m.hasDeputy() {
		return
	}

	for _, player := range m.Players.Players {
		if player.GetRole() == pb.NodeRole_NORMAL {
			m.assignDeputy(player)
			break
		}
	}
}

// проверка наличия Deputy
func (m *Master) hasDeputy() bool {
	for _, player := range m.Players.Players {
		if player.GetRole() == pb.NodeRole_DEPUTY {
			return true
		}
	}
	return false
}

func chooseDirection(headX int32, headY int32, tailX int32, tailY int32) pb.Direction {
	if headX == tailX {
		if headY-1 == tailY {
			return pb.Direction_DOWN
		} else if headY+1 == tailY {
			return pb.Direction_UP
		}
	} else if headY == tailY {
		if headX-1 == tailX {
			return pb.Direction_LEFT
		} else if headY+1 == tailY {
			return pb.Direction_RIGHT
		}
	}
	return pb.Direction_UP
}

func (m *Master) chooseTailCell(head *pb.GameState_Coord) (*pb.GameState_Coord, pb.Direction) {
	foods := m.Node.State.Foods
	for _, food := range foods {
		x, y := food.X, food.Y
		coord := &pb.GameState_Coord{X: x, Y: y}
		if *x == head.GetX() && (*y == head.GetY()-1 || *y == head.GetY()+1) {
			continue
		} else if *y == head.GetY() && (*x == head.GetX()-1 || *x == head.GetX()+1) {
			continue
		}
		dir := chooseDirection(head.GetX(), head.GetY(), head.GetX(), head.GetY())
		return coord, dir
	}
	return nil, pb.Direction_UP
}

func (m *Master) addSnakeForNewPlayer(playerID int32, coord *pb.GameState_Coord) {
	tail, dir := m.chooseTailCell(coord)
	newSnake := &pb.GameState_Snake{
		PlayerId: proto.Int32(playerID),
		Points: []*pb.GameState_Coord{
			{
				X: proto.Int32(coord.GetX()),
				Y: proto.Int32(coord.GetY()),
			},
			{
				X: tail.X,
				Y: tail.Y,
			},
		},
		State:         pb.GameState_Snake_ALIVE.Enum(),
		HeadDirection: dir.Enum(),
	}

	m.Node.State.Snakes = append(m.Node.State.Snakes, newSnake)
}

func (m *Master) handleDiscoverMessage(addr *net.UDPAddr) {
	log.Printf("Received DiscoverMsg from %v via unicast", addr)
	announcementMsg := &pb.GameMessage{
		MsgSeq: proto.Int64(m.Node.MsgSeq),
		Type: &pb.GameMessage_Announcement{
			Announcement: &pb.GameMessage_AnnouncementMsg{
				Games: []*pb.GameAnnouncement{m.announcement},
			},
		},
	}

	m.Node.SendMessage(announcementMsg, addr)
}

func (m *Master) handleSteerMessage(steerMsg *pb.GameMessage_SteerMsg, playerId int32) {
	var snake *pb.GameState_Snake
	for _, s := range m.Node.State.Snakes {
		if s.GetPlayerId() == playerId {
			snake = s
			break
		}
	}

	if snake == nil {
		log.Printf("No snake found for player ID: %d", playerId)
		return
	}

	newDirection := steerMsg.GetDirection()
	currentDirection := snake.GetHeadDirection()

	isOppositeDirection := func(cur, new pb.Direction) bool {
		switch cur {
		case pb.Direction_UP:
			return new == pb.Direction_DOWN
		case pb.Direction_DOWN:
			return new == pb.Direction_UP
		case pb.Direction_LEFT:
			return new == pb.Direction_RIGHT
		case pb.Direction_RIGHT:
			return new == pb.Direction_LEFT
		}
		return false
	}(currentDirection, newDirection)

	if isOppositeDirection {
		log.Printf("Invalid direction change from player ID: %d", playerId)
		return
	}

	snake.HeadDirection = newDirection.Enum()
	log.Printf("Player ID: %d changed direction to: %v", playerId, newDirection)
}

// Обработка отвалившихся узлов
func (m *Master) checkTimeouts() {
	ticker := time.NewTicker(time.Duration(0.8*float64(m.Node.Config.GetStateDelayMs())) * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		m.Node.LastInteraction.Range(func(key, value interface{}) bool {
			playerId, lastInteraction := key.(int32), value.(time.Time)
			if playerId == m.Node.PlayerInfo.GetId() {
				return true
			}
			if now.Sub(lastInteraction) > time.Duration(0.8*float64(m.Node.Config.GetStateDelayMs()))*time.Millisecond {
				log.Printf("player ID: %d has timeout", playerId)
				m.removePlayer(playerId)
			}
			return true
		})
	}
}

func (m *Master) removePlayer(playerId int32) {
	m.Node.LastInteraction.Delete(playerId)

	var removedPlayer *pb.GamePlayer
	var index int
	for i, player := range m.Players.Players {
		if player.GetId() == playerId {
			removedPlayer = player
			index = i
			break
		}
	}

	if removedPlayer == nil {
		log.Printf("Player ID: %d not found for removal", playerId)
		return
	}

	// Удаляем, только если игрок не VIEWER
	if removedPlayer.GetRole() != pb.NodeRole_VIEWER {
		m.Players.Players = append(m.Players.Players[:index], m.Players.Players[index+1:]...)
		m.Node.LastSent.Delete(fmt.Sprintf("%s:%d", removedPlayer.GetIpAddress(), removedPlayer.GetPort()))
		m.Node.State.Players = m.Players
	}

	// Если игрок был DEPUTY, назначаем нового
	if removedPlayer.GetRole() == pb.NodeRole_DEPUTY {
		m.findNewDeputy()
	}

	// Если игрок был MASTER, назначаем нового
	if removedPlayer.GetRole() == pb.NodeRole_MASTER && m.Node.PlayerInfo.GetId() == removedPlayer.GetId() {
		//fmt.Printf("------------------- well, im here..... --------------------")
		//debug.PrintStack()
		m.makeNewMaster()
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}

	// Если игрок стал VIEWER, переводим его змейку в ZOMBIE
	if removedPlayer.GetRole() == pb.NodeRole_VIEWER {
		m.MakeSnakeZombie(playerId)
	}

	log.Printf("Player ID: %d processed for removal or role change", playerId)

	m.Node.State.Players = m.Players
}

func (m *Master) makeNewMaster() {
	for _, player := range m.Players.Players {
		if player.GetRole() == pb.NodeRole_DEPUTY {
			m.assignMaster(player)
			break
		}
	}
}

// Назначение нового мастера
func (m *Master) assignMaster(player *pb.GamePlayer) {
	/*player.Role = pb.NodeRole_MASTER.Enum()*/

	roleChangeMsg := &pb.GameMessage{
		MsgSeq:     proto.Int64(m.Node.MsgSeq),
		SenderId:   proto.Int32(m.Node.PlayerInfo.GetId()),
		ReceiverId: proto.Int32(player.GetId()),
		Type: &pb.GameMessage_RoleChange{
			RoleChange: &pb.GameMessage_RoleChangeMsg{
				SenderRole:   pb.NodeRole_MASTER.Enum(),
				ReceiverRole: pb.NodeRole_MASTER.Enum(),
			},
		},
	}
	playerAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", player.GetIpAddress(), player.GetPort()))
	if err != nil {
		log.Printf("Error resolving address for Deputy: %v", err)
		return
	}

	m.Node.SendMessage(roleChangeMsg, playerAddr)
	log.Printf("Player ID: %d is assigned as MASTER", player.GetId())
}

func (m *Master) findNewDeputy() {
	for _, player := range m.Players.Players {
		if player.GetRole() == pb.NodeRole_NORMAL {
			m.assignDeputy(player)
			break
		}
	}
}

// Назначение нового заместителя
func (m *Master) assignDeputy(player *pb.GamePlayer) {
	player.Role = pb.NodeRole_DEPUTY.Enum()

	roleChangeMsg := &pb.GameMessage{
		MsgSeq:     proto.Int64(m.Node.MsgSeq),
		SenderId:   proto.Int32(m.Node.PlayerInfo.GetId()),
		ReceiverId: proto.Int32(player.GetId()),
		Type: &pb.GameMessage_RoleChange{
			RoleChange: &pb.GameMessage_RoleChangeMsg{
				SenderRole:   pb.NodeRole_MASTER.Enum(),
				ReceiverRole: pb.NodeRole_DEPUTY.Enum(),
			},
		},
	}
	playerAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", player.GetIpAddress(), player.GetPort()))
	if err != nil {
		log.Printf("Error resolving address for Deputy: %v", err)
		return
	}

	m.Node.SendMessage(roleChangeMsg, playerAddr)
	log.Printf("Player ID: %d is assigned as DEPUTY", player.GetId())
}

func (m *Master) MakeSnakeZombie(playerId int32) {
	for _, snake := range m.Node.State.Snakes {
		if snake.GetPlayerId() == playerId {
			snake.State = pb.GameState_Snake_ZOMBIE.Enum()
			log.Printf("Snake for player ID: %d is now a ZOMBIE", playerId)
			return
		}
	}
	log.Printf("No snake found for player ID: %d to make ZOMBIE", playerId)
}

func (m *Master) handleRoleChangeMessage(msg *pb.GameMessage, addr *net.UDPAddr) {
	roleChangeMsg := msg.GetRoleChange()

	switch {
	case roleChangeMsg.GetSenderRole() == pb.NodeRole_NORMAL && roleChangeMsg.GetReceiverRole() == pb.NodeRole_DEPUTY:
		// NORMAL -> DEPUTY
		m.checkAndAssignDeputy() // если ещё нет заместителя, даём узлу стать заместителем

	/*case roleChangeMsg.GetSenderRole() == pb.NodeRole_DEPUTY && roleChangeMsg.GetReceiverRole() == pb.NodeRole_MASTER:
	// DEPUTY -> MASTER
	log.Printf("Deputy has taken over as MASTER. Stopping PlayerInfo.")
	m.stopMaster()*/

	case roleChangeMsg.GetReceiverRole() == pb.NodeRole_VIEWER:
		// Player -> VIEWER
		playerId := msg.GetSenderId()
		log.Printf("Player ID: %d is now a VIEWER. Converting snake to ZOMBIE.", playerId)
		m.MakeSnakeZombie(playerId)

		for _, player := range m.Players.Players {
			if player.GetId() == playerId {
				player.Role = pb.NodeRole_VIEWER.Enum()
				break
			}
		}
	default:
		log.Printf("Received unknown RoleChangeMsg from player ID: %d", msg.GetSenderId())
	}
}

/*func (m *Master) stopMaster() {
	log.Println("Switching PlayerInfo role to VIEWER...")

	m.Node.PlayerInfo.Role = pb.NodeRole_VIEWER.Enum()
	m.MakeSnakeZombie(m.Node.PlayerInfo.GetId())

	m.announcement.CanJoin = proto.Bool(false)
	log.Println("Master is now a VIEWER. Continuing as an observer.")
}*/
