package player

import (
	pb "SnakeGame/model/proto"
	"google.golang.org/protobuf/proto"
	"log"
	"net"
	"os"
	"time"
)

func (p *Player) handleMulticastMessage(msg *pb.GameMessage, addr *net.UDPAddr) {
	switch t := msg.Type.(type) {
	case *pb.GameMessage_Announcement:
		for _, game := range t.Announcement.Games {
			p.addDiscoveredGame(game, addr, t.Announcement)
		}
	default:
	}
}

func (p *Player) handleUnicastMessage(msg *pb.GameMessage, addr *net.UDPAddr) {
	p.Node.Mu.Lock()
	defer p.Node.Mu.Unlock()

	p.Node.LastInteraction.Store(msg.GetSenderId(), time.Now())

	switch t := msg.Type.(type) {
	case *pb.GameMessage_Ack:
		if !p.haveId {
			p.Node.PlayerInfo.Id = proto.Int32(msg.GetReceiverId())
			log.Printf("Joined game with ID: %d", p.Node.PlayerInfo.GetId())
			p.haveId = true
		}
		p.Node.AckChan <- msg.GetMsgSeq()
	case *pb.GameMessage_Announcement:
		p.MasterAddr = addr
		p.Node.MasterAddr = addr
		p.AnnouncementMsg = t.Announcement
		log.Printf("Received AnnouncementMsg from %v via unicast", addr)
		p.sendJoinRequest()
	case *pb.GameMessage_State:
		if t.State.GetState().GetStateOrder() <= p.LastStateMsg {
			return
		} else {
			p.LastStateMsg = t.State.GetState().GetStateOrder()
		}
		p.Node.State = t.State.GetState()
		p.Node.SendAck(msg, addr)
		p.Node.Cond.Broadcast()
	case *pb.GameMessage_Error:
		p.Node.SendAck(msg, addr)
		if t.Error.GetErrorMessage() == "You have crashed and been removed from the game. Exiting..." {
			log.Printf("Received crash notification: %s", t.Error.GetErrorMessage())
			os.Exit(0)
		}
	case *pb.GameMessage_RoleChange:
		p.handleRoleChangeMessage(msg)
		p.Node.SendAck(msg, addr)
	case *pb.GameMessage_Ping:
		// Отправляем AckMsg в ответ
		p.Node.SendAck(msg, addr)
	default:
		log.Printf("Received unknown message")
	}
}

func (p *Player) handleRoleChangeMessage(msg *pb.GameMessage) {
	roleChangeMsg := msg.GetRoleChange()
	switch {
	case roleChangeMsg.GetReceiverRole() == pb.NodeRole_DEPUTY:
		// DEPUTY
		p.Node.PlayerInfo.Role = pb.NodeRole_DEPUTY.Enum()
		log.Printf("Assigned as DEPUTY")
	case roleChangeMsg.GetReceiverRole() == pb.NodeRole_MASTER:
		// MASTER
		p.becomeMaster(true)
		log.Printf("Assigned as MASTER")
	case roleChangeMsg.GetReceiverRole() == pb.NodeRole_VIEWER:
		// VIEWER
		p.Node.PlayerInfo.Role = pb.NodeRole_VIEWER.Enum()
		log.Printf("Assigned as VIEWER")
	default:
		log.Printf("Received unknown RoleChangeMsg")
	}
}
