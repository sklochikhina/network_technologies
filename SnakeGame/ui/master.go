package ui

import (
	"SnakeGame/model/common"
	"SnakeGame/model/master"
	pb "SnakeGame/model/proto"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"google.golang.org/protobuf/proto"
	"math/rand"
	"net"
	"strconv"
	"time"
)

func CheckInputValues(widthEntry *widget.Entry, heightEntry *widget.Entry, foodEntry *widget.Entry, delayEntry *widget.Entry) (int, int, int, int, error) {
	width, err1 := strconv.Atoi(widthEntry.Text)
	height, err2 := strconv.Atoi(heightEntry.Text)
	food, err3 := strconv.Atoi(foodEntry.Text)
	delay, err4 := strconv.Atoi(delayEntry.Text)

	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return 0, 0, 0, 0, fmt.Errorf("все поля должны быть числами")
	}

	if width < 10 || width > 100 {
		return 0, 0, 0, 0, fmt.Errorf("ширина поля должна быть от 10 до 100")
	}
	if height < 10 || height > 100 {
		return 0, 0, 0, 0, fmt.Errorf("высота поля должна быть от 10 до 100")
	}
	if food < 0 || food > 100 {
		return 0, 0, 0, 0, fmt.Errorf("количество еды должно быть от 0 до 100")
	}
	if delay < 100 || delay > 3000 {
		return 0, 0, 0, 0, fmt.Errorf("задержка должна быть от 100 до 3000 мс")
	}

	return width, height, food, delay, nil
}

// ShowGameConfig настройки игры
func ShowGameConfig(w fyne.Window, multConn *net.UDPConn) {
	widthEntry := widget.NewEntry() // Ширина поля в клетках (от 10 до 100)
	widthEntry.SetText("25")

	heightEntry := widget.NewEntry() // Высота поля в клетках (от 10 до 100)
	heightEntry.SetText("25")

	foodEntry := widget.NewEntry() // Количество клеток с едой, независимо от числа игроков (от 0 до 100)
	foodEntry.SetText("10")

	delayEntry := widget.NewEntry() // Задержка между ходами (сменой состояний) в игре, в миллисекундах (от 100 до 3000)
	delayEntry.SetText("180")

	startButton := widget.NewButton("Начать игру", func() {
		width, height, food, delay, err := CheckInputValues(widthEntry, heightEntry, foodEntry, delayEntry)
		if err != nil {
			dialog := widget.NewLabel(fmt.Sprintf("Ошибка: %v", err))
			dialog.Alignment = fyne.TextAlignCenter

			okButton := widget.NewButton("Ок", func() {
				ShowGameConfig(w, multConn)
			})

			errorContent := container.NewVBox(
				dialog,
				okButton,
			)
			w.SetContent(container.NewCenter(errorContent))

			return
		}

		config := &pb.GameConfig{
			Width:        proto.Int32(int32(width)),
			Height:       proto.Int32(int32(height)),
			FoodStatic:   proto.Int32(int32(food)),
			StateDelayMs: proto.Int32(int32(delay)),
		}

		ShowMasterGameScreen(w, config, multConn)
	})

	backButton := widget.NewButton("Назад", func() {
		ShowMainMenu(w, multConn)
	})

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Ширина поля", Widget: widthEntry},
			{Text: "Высота поля", Widget: heightEntry},
			{Text: "Количество еды", Widget: foodEntry},
			{Text: "Задержка (мс)", Widget: delayEntry},
		},
	}

	content := container.NewVBox(
		widget.NewLabelWithStyle("Настройки игры", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		form,
		startButton,
		backButton,
	)

	w.SetContent(container.NewCenter(content))
}

// ShowMasterGameScreen показывает экран игры
func ShowMasterGameScreen(w fyne.Window, config *pb.GameConfig, multConn *net.UDPConn) {
	// Создаём мастера
	masterNode := master.NewMaster(multConn, config)
	go masterNode.Start()

	// Подготавливаем поле
	gameContent := CreateGameContent(config)

	scoreLabel := widget.NewLabel("Счёт: 0")
	nameLabel := widget.NewLabel("Имя: ")
	roleLabel := widget.NewLabel("Роль: ")

	// Подготавливаем информационную панель
	infoPanel, scoreTable, foodCountLabel := createInfoPanel(config, func() {
		StopGameLoop() // будет вызвана при нажатии на "Выйти"
		ShowMainMenu(w, multConn)
	}, scoreLabel, nameLabel, roleLabel)

	// Делим окно на поле и информационную панель
	splitContent := container.NewHSplit(
		gameContent,
		infoPanel,
	)
	splitContent.SetOffset(0.7)

	w.SetContent(splitContent)

	StartGameLoopForMaster(w, masterNode.Node, gameContent, scoreTable, foodCountLabel,
		func(score int32) { scoreLabel.SetText(fmt.Sprintf("Счёт: %d", score)) },
		func(name string) { nameLabel.SetText(fmt.Sprintf("Имя: %v", name)) },
		func(role pb.NodeRole) { roleLabel.SetText(fmt.Sprintf("Роль: %v", role)) },
	)
}

func StartGameLoopForMaster(w fyne.Window, node *common.Node, gameContent *fyne.Container, scoreTable *widget.Table, foodCountLabel *widget.Label,
	updateScore func(int32), updateName func(string), updateRole func(pb.NodeRole)) {

	rand.NewSource(time.Now().UnixNano())

	gameTicker = time.NewTicker(time.Millisecond * 60)
	isRunning = true

	// Обработка нажатия клавиш
	w.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		handleKeyInputForMaster(e, node)
	})

	// Ждём, пока состояние не инициализируется
	if node.State == nil {
		node.Mu.Lock()
		for node.State == nil {
			node.Cond.Wait()
		}
		node.Mu.Unlock()
	}

	// Обновление поля и информационной панели
	go func() {
		for isRunning {
			select {
			case <-gameTicker.C:
				node.Mu.Lock()
				stateCopy := proto.Clone(node.State).(*pb.GameState)
				configCopy := proto.Clone(node.Config).(*pb.GameConfig)
				// Обновление счёта
				var playerScore int32
				for _, gamePlayer := range node.State.GetPlayers().GetPlayers() {
					if gamePlayer.GetId() == node.PlayerInfo.GetId() {
						playerScore = gamePlayer.GetScore()
						break
					}
				}
				updateScore(playerScore)
				updateName(node.PlayerInfo.GetName())
				updateRole(pb.NodeRole_MASTER)
				renderGameState(gameContent, stateCopy, configCopy)
				updateInfoPanel(scoreTable, foodCountLabel, stateCopy)
				node.Mu.Unlock()
			}
		}
	}()
}

// handleKeyInput обработка нажатия клавиш для мастера
func handleKeyInputForMaster(e *fyne.KeyEvent, node *common.Node) {
	var newDirection pb.Direction

	switch e.Name {
	case fyne.KeyW, fyne.KeyUp:
		newDirection = pb.Direction_UP
	case fyne.KeyS, fyne.KeyDown:
		newDirection = pb.Direction_DOWN
	case fyne.KeyA, fyne.KeyLeft:
		newDirection = pb.Direction_LEFT
	case fyne.KeyD, fyne.KeyRight:
		newDirection = pb.Direction_RIGHT
	default:
		return
	}

	node.Mu.Lock()
	defer node.Mu.Unlock()

	for _, snake := range node.State.Snakes {
		if snake.GetPlayerId() == node.PlayerInfo.GetId() {
			currentDirection := snake.GetHeadDirection()
			// Проверка направления
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
			}(currentDirection, newDirection) // объявили функцию и сразу вызываем (Immediately Invoked Function Expression)

			if !isOppositeDirection {
				snake.HeadDirection = newDirection.Enum()
			}
			break
		}
	}
}
