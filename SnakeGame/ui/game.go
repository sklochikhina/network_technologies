package ui

import (
	"SnakeGame/model/common"
	pb "SnakeGame/model/proto"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"image/color"
)

const CellSize = 20

// renderGameState выводит игру на экран
func renderGameState(content *fyne.Container, state *pb.GameState, config *pb.GameConfig) {
	content.Objects = nil // Очищаем старое содержимое поля

	// Игровое поле
	for i := int32(0); i < config.GetWidth(); i++ {
		for j := int32(0); j < config.GetHeight(); j++ {
			cell := canvas.NewRectangle(color.RGBA{R: 50, G: 50, B: 50, A: 255})
			cell.StrokeColor = color.RGBA{R: 0, G: 0, B: 0, A: 255} // цвет разделителя клеток
			cell.StrokeWidth = 1                                    // ширина разделителя
			cell.Resize(fyne.NewSize(CellSize, CellSize))
			cell.Move(fyne.NewPos(float32(i)*CellSize, float32(j)*CellSize))
			content.Add(cell)
		}
	}

	// Еда
	for _, food := range state.Foods {
		apple := canvas.NewCircle(color.RGBA{255, 128, 0, 255})
		apple.Resize(fyne.NewSize(CellSize, CellSize))
		x := float32(food.GetX()) * CellSize
		y := float32(food.GetY()) * CellSize
		apple.Move(fyne.NewPos(x, y))
		content.Add(apple)
	}

	// Змеи
	for _, snake := range state.Snakes {
		for i, point := range snake.Points {
			var rect *canvas.Rectangle
			role := getUserById(snake.GetPlayerId(), state)
			snakeState := common.GetSnakeStateById(snake.GetPlayerId(), state)
			if snakeState == pb.GameState_Snake_ALIVE {
				if i == 0 {
					// голова
					switch role {
					case pb.NodeRole_MASTER:
						rect = canvas.NewRectangle(color.RGBA{255, 0, 0, 255})
					case pb.NodeRole_NORMAL:
						rect = canvas.NewRectangle(color.RGBA{0, 255, 0, 255})
					case pb.NodeRole_DEPUTY:
						rect = canvas.NewRectangle(color.RGBA{150, 90, 255, 255})
					}
				} else {
					// тело
					switch role {
					case pb.NodeRole_MASTER:
						rect = canvas.NewRectangle(color.RGBA{R: 128, A: 255})
					case pb.NodeRole_NORMAL:
						rect = canvas.NewRectangle(color.RGBA{0, 128, 0, 255})
					case pb.NodeRole_DEPUTY:
						rect = canvas.NewRectangle(color.RGBA{120, 60, 200, 255})
					}
				}
			} else {
				if i == 0 {
					// голова зомби
					rect = canvas.NewRectangle(color.RGBA{50, 0, 80, 255})
				} else {
					// тело зомби
					rect = canvas.NewRectangle(color.RGBA{70, 0, 100, 255})
				}
			}
			rect.Resize(fyne.NewSize(CellSize, CellSize))
			x := float32(point.GetX()) * CellSize
			y := float32(point.GetY()) * CellSize
			rect.Move(fyne.NewPos(x, y))
			content.Add(rect)
		}
	}

	// Перерисовываем поле и его содержимое
	content.Refresh()
}

// Получение роли игрока по id
func getUserById(id int32, state *pb.GameState) pb.NodeRole {
	for _, player := range state.Players.Players {
		if player.GetId() == id {
			return player.GetRole()
		}
	}
	return pb.NodeRole_VIEWER
}
