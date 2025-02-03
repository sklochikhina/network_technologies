package master

import (
	pb "SnakeGame/model/proto"
	"fmt"
	"google.golang.org/protobuf/proto"
	"log"
	"math/rand"
	"net"
)

// GenerateFood генерация еды
func (m *Master) GenerateFood() {
	requireFood := m.Node.Config.GetFoodStatic() + int32(len(m.Node.State.Snakes)) // food_static + (число ALIVE-змеек)
	currentFood := int32(len(m.Node.State.GetFoods()))

	if currentFood < requireFood {
		needNum := requireFood - currentFood
		for i := int32(0); i < needNum; i++ {
			coord := m.findEmptyCell()
			if coord != nil {
				m.Node.State.Foods = append(m.Node.State.Foods, coord)
			} else {
				log.Println("No empty cells available for new food.")
				break
			}
		}
	}
}

func (m *Master) findEmptyCell() *pb.GameState_Coord {
	numCells := m.Node.Config.GetWidth() * m.Node.Config.GetHeight()
	for attempts := int32(0); attempts < numCells; attempts++ {
		x := rand.Int31n(m.Node.Config.GetWidth())
		y := rand.Int31n(m.Node.Config.GetHeight())
		if m.isCellEmpty(x, y) {
			return &pb.GameState_Coord{X: proto.Int32(x), Y: proto.Int32(y)}
		}
	}
	return nil
}

func (m *Master) isCellEmpty(x, y int32) bool {
	for _, snake := range m.Node.State.Snakes {
		for _, point := range snake.Points {
			if point.GetX() == x && point.GetY() == y {
				return false
			}
		}
	}

	for _, food := range m.Node.State.Foods {
		if food.GetX() == x && food.GetY() == y {
			return false
		}
	}

	return true
}

// UpdateGameState обновление состояния игры
func (m *Master) UpdateGameState() {
	for _, snake := range m.Node.State.Snakes {
		m.moveSnake(snake)
	}
	m.checkCollisions()
}

func (m *Master) moveSnake(snake *pb.GameState_Snake) {
	head := snake.Points[0]
	newHead := &pb.GameState_Coord{
		X: proto.Int32(head.GetX()),
		Y: proto.Int32(head.GetY()),
	}

	// Изменение координат
	switch snake.GetHeadDirection() {
	case pb.Direction_UP:
		newHead.Y = proto.Int32(newHead.GetY() - 1)
	case pb.Direction_DOWN:
		newHead.Y = proto.Int32(newHead.GetY() + 1)
	case pb.Direction_LEFT:
		newHead.X = proto.Int32(newHead.GetX() - 1)
	case pb.Direction_RIGHT:
		newHead.X = proto.Int32(newHead.GetX() + 1)
	}

	// Поведение при столкновении со стеной
	if newHead.GetX() < 0 {
		newHead.X = proto.Int32(m.Node.Config.GetWidth() - 1)
	} else if newHead.GetX() >= m.Node.Config.GetWidth() {
		newHead.X = proto.Int32(0)
	}
	if newHead.GetY() < 0 {
		newHead.Y = proto.Int32(m.Node.Config.GetHeight() - 1)
	} else if newHead.GetY() >= m.Node.Config.GetHeight() {
		newHead.Y = proto.Int32(0)
	}

	// Добавляем новую голову
	snake.Points = append([]*pb.GameState_Coord{newHead}, snake.Points...)
	if !m.isFoodEaten(newHead) {
		snake.Points = snake.Points[:len(snake.Points)-1]
	} else {
		// Игрок заработал +1 балл
		snakeId := snake.GetPlayerId()
		for _, player := range m.Players.GetPlayers() {
			if player.GetId() == snakeId {
				player.Score = proto.Int32(player.GetScore() + 1)
				break
			}
		}
	}
}

func (m *Master) isFoodEaten(head *pb.GameState_Coord) bool {
	for i, food := range m.Node.State.Foods {
		if head.GetX() == food.GetX() && head.GetY() == food.GetY() {
			m.Node.State.Foods = append(m.Node.State.Foods[:i], m.Node.State.Foods[i+1:]...)
			return true
		}
	}
	return false
}

// Проверяем столкновения с другими змейками
func (m *Master) checkCollisions() {
	heads := make(map[string]int32)

	for _, snake := range m.Node.State.Snakes {
		head := snake.Points[0]
		point := fmt.Sprintf("%d,%d", head.GetX(), head.GetY())
		heads[point] = snake.GetPlayerId()
	}

	// Проверяем, есть ли клетки с более чем одной головой
	for key := range heads {
		count := 0 // количество голов на одной клетке
		var crashedPlayers []int32
		for k, pid := range heads {
			if k == key {
				count++
				crashedPlayers = append(crashedPlayers, pid)
			}
		}
		if count > 1 { // Несколько голов на одной клетке - все погибают
			for _, pid := range crashedPlayers {
				m.killSnake(pid, pid)
			}
		}
	}

	// Проверяем столкновения головы змейки с телом других змей
	for _, snake := range m.Node.State.Snakes {
		head := snake.Points[0]
		headX, headY := head.GetX(), head.GetY()
		for _, otherSnake := range m.Node.State.Snakes {
			for i, point := range otherSnake.Points {
				// Если это собственная змейка и это голова, пропускаем
				if otherSnake.GetPlayerId() == snake.GetPlayerId() && i == 0 {
					continue
				}
				if otherSnake.GetState() == pb.GameState_Snake_ZOMBIE {
					log.Printf("Snake with player ID %d is a zombi!!!!!!!!!!!!!!!!!!", otherSnake.GetPlayerId())
				}
				if point.GetX() == headX && point.GetY() == headY {
					for _, player := range m.Players.GetPlayers() {
						if player.GetId() == snake.GetPlayerId() {
							log.Printf("Player ID %d needs to be KILLED\n", snake.GetPlayerId())
							m.killSnake(snake.GetPlayerId(), otherSnake.GetPlayerId())
						}
					}
				}
			}
		}
	}
}

// Убираем умершую змею
func (m *Master) killSnake(crashedPlayerId, killer int32) {
	m.Node.Mu.Lock()
	defer m.Node.Mu.Unlock()

	var indexToRemove int
	var snakeToRemove *pb.GameState_Snake
	for index, snake := range m.Node.State.Snakes {
		if snake.GetPlayerId() == crashedPlayerId {
			indexToRemove = index
			snakeToRemove = snake
			break
		}
	}

	if snakeToRemove != nil {
		for _, point := range snakeToRemove.Points {
			if rand.Float32() < 0.5 {
				// Заменяем на еду
				newFood := &pb.GameState_Coord{
					X: proto.Int32(point.GetX()),
					Y: proto.Int32(point.GetY()),
				}
				m.Node.State.Foods = append(m.Node.State.Foods, newFood)
			}
		}
		//m.Node.State.Snakes[indexToRemove].State = pb.GameState_Snake_ZOMBIE.Enum()
		m.Node.State.Snakes = append(m.Node.State.Snakes[:indexToRemove], m.Node.State.Snakes[indexToRemove+1:]...)
	}

	// Проверяем, не убил ли игрок сам себя
	if crashedPlayerId != killer {
		for _, player := range m.Players.Players {
			if player.GetId() == killer {
				// Убийце +1 балл
				player.Score = proto.Int32(player.GetScore() + 1)
				break
			}
		}
	}

	if crashedPlayerId != m.Node.PlayerInfo.GetId() {
		// Сохраняем адрес игрока, чтобы отправить ему ErrorMsg
		var crashedPlayerAddr *net.UDPAddr
		for _, player := range m.Players.Players {
			if player.GetId() == crashedPlayerId {
				addrStr := fmt.Sprintf("%s:%d", player.GetIpAddress(), player.GetPort())
				addr, err := net.ResolveUDPAddr("udp", addrStr)
				if err == nil {
					crashedPlayerAddr = addr
				}
				break
			}
		}

		m.removePlayer(crashedPlayerId)
		log.Printf("Player ID: %d has crashed and been removed.", crashedPlayerId)

		// Отправляем ErrorMsg упавшему игроку, чтобы он у себя вызвал os.Exit(0)
		if crashedPlayerAddr != nil {
			errorMsg := &pb.GameMessage{
				MsgSeq: proto.Int64(m.Node.MsgSeq),
				Type: &pb.GameMessage_Error{
					Error: &pb.GameMessage_ErrorMsg{
						ErrorMessage: proto.String("You have crashed and been removed from the game. Exiting..."),
					},
				},
			}
			m.Node.SendMessage(errorMsg, crashedPlayerAddr)
		}
	} else {
		m.removePlayer(crashedPlayerId)
	}
}

func (m *Master) hasFreeSquare(state *pb.GameState, config *pb.GameConfig, squareSize int32) (bool, *pb.GameState_Coord) {
	occupied := make([][]bool, config.GetWidth())
	for i := range occupied {
		occupied[i] = make([]bool, config.GetHeight())
	}

	// Проходимся по всем змейкам и записываем их текущие ключевые точки в occupied
	for _, snake := range state.Snakes {
		for _, point := range snake.Points {
			x, y := point.GetX(), point.GetY()
			if x >= 0 && x < config.GetWidth() && y >= 0 && y < config.GetHeight() {
				occupied[x][y] = true
			}
		}
	}

	// Проверяем все квадраты размером squareSize (без учёта тора)
	for startX := int32(0); startX <= config.GetWidth()-squareSize; startX++ {
		for startY := int32(0); startY <= config.GetHeight()-squareSize; startY++ {
			if isSquareFree(occupied, state.GetFoods(), startX, startY, squareSize) {
				return true, &pb.GameState_Coord{X: proto.Int32(startX), Y: proto.Int32(startY)}
			}
		}
	}

	return false, nil
}

func isSquareFree(occupied [][]bool, foods []*pb.GameState_Coord, startX, startY, squareSize int32) bool {
	for x := startX; x < startX+squareSize; x++ {
		for y := startY; y < startY+squareSize; y++ {
			if occupied[x][y] {
				return false
			}
		}
	}

	// Проверяем, не заняты ли центральная клетка и её соседние едой
	squareFoods := make([][]bool, squareSize)
	for i := range squareFoods {
		squareFoods[i] = make([]bool, squareSize)
	}

	for _, food := range foods {
		x, y := food.GetX(), food.GetY()
		if x >= startX && x < startX+squareSize && y >= startY && y < startY+squareSize {
			squareFoods[x-startX][y-startY] = true
		}
	}

	if squareFoods[2][2] { // если едой занята центральная клетка - ищем дальше
		return false
	} else if squareFoods[1][2] && squareFoods[2][1] &&
		squareFoods[2][3] && squareFoods[3][2] { // если все клетки вокруг центра заняты едой - ищем дальше
		return false
	}

	return true
}
