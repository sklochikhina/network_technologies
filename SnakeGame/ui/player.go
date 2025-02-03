package ui

import (
	"SnakeGame/model/player"
	pb "SnakeGame/model/proto"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"google.golang.org/protobuf/proto"
	"log"
	"math/rand"
	"net"
	"time"
)

// ShowJoinGame отображает экран присоединения к игре
func ShowJoinGame(w fyne.Window, multConn *net.UDPConn, role pb.NodeRole) {
	log.Printf("присоединение...")
	playerNode := player.NewPlayer(multConn, role)

	// Запускаем получение мультикаст-сообщений
	go playerNode.ReceiveMulticastMessages()

	discoveryLabel := widget.NewLabel("Поиск доступных игр...")
	discoveryLabel.Alignment = fyne.TextAlignCenter

	// Список идущих игр
	gameList := widget.NewSelect([]string{}, func(value string) {
		log.Printf("Selected game: %s", value)
	})
	gameList.PlaceHolder = "Выберите игру"
	gameList.Resize(fyne.NewSize(300, 50))

	// Ввод имени пользователя
	playerNameEntry := widget.NewEntry()
	playerNameEntry.SetPlaceHolder("Введите ваше имя")

	joinButton := widget.NewButton("Присоединиться", func() {
		playerName := playerNameEntry.Text
		if playerName == "" {
			dialog := widget.NewLabel(fmt.Sprintf("Имя игрока не может быть пустым."))
			dialog.Alignment = fyne.TextAlignCenter

			okButton := widget.NewButton("Ок", func() {
				ShowJoinGame(w, multConn, role)
			})

			errorContent := container.NewVBox(
				dialog,
				okButton,
			)
			w.SetContent(container.NewCenter(errorContent))
			return
		}

		// Получаем выбранную игру из списка
		selectedGame := getSelectedGame(playerNode, gameList)
		if selectedGame != nil {
			ShowPlayerGameScreen(w, playerNode, playerName, selectedGame, multConn)
		}
	})

	backButton := widget.NewButton("Назад", func() {
		ShowMainMenu(w, multConn)
	})

	content := container.NewVBox(
		discoveryLabel,
		gameList,
		widget.NewForm(
			&widget.FormItem{Text: "Имя игрока", Widget: playerNameEntry},
		),
		joinButton,
		backButton,
	)

	w.SetContent(container.NewCenter(content))

	// Реализуем обнаружение игр и обновление списка
	go func() {
		for {
			time.Sleep(500 * time.Millisecond) // Интервал обновления
			games := playerNode.DiscoveredGames
			gameList.Options = getGameNames(games)
			gameList.Refresh()
		}
	}()
}

func getGameNames(games []player.DiscoveredGame) []string {
	names := make([]string, len(games))
	for i, game := range games {
		names[i] = game.GameName
	}
	return names
}

func getSelectedGame(playerNode *player.Player, gameList *widget.Select) *player.DiscoveredGame {
	for _, game := range playerNode.DiscoveredGames {
		if gameList.Selected == game.GameName {
			return &game
		}
	}
	log.Printf("Could't find selected game")
	return nil
}

// ShowPlayerGameScreen инициализирует игрока и запускает UI игры
func ShowPlayerGameScreen(w fyne.Window, playerNode *player.Player, playerName string,
	selectedGame *player.DiscoveredGame, multConn *net.UDPConn) {

	playerNode.Node.PlayerInfo.Name = proto.String(playerName)
	playerNode.Node.GameName = selectedGame.GameName
	playerNode.Node.Config = selectedGame.Config
	playerNode.MasterAddr = selectedGame.MasterAddr
	playerNode.AnnouncementMsg = selectedGame.AnnouncementMsg
	playerNode.Start()

	gameContent := CreateGameContent(playerNode.Node.Config)

	scoreLabel := widget.NewLabel("Счет: 0")
	nameLabel := widget.NewLabel("Имя: ")
	roleLabel := widget.NewLabel("Роль: ")
	infoPanel, scoreTable, foodCountLabel := createInfoPanel(playerNode.Node.Config, func() {
		StopGameLoop()
		ShowMainMenu(w, multConn)
	}, scoreLabel, nameLabel, roleLabel)

	splitContent := container.NewHSplit(
		gameContent,
		infoPanel,
	)
	splitContent.SetOffset(0.7)

	w.SetContent(splitContent)

	StartGameLoopForPlayer(w, playerNode, gameContent, scoreTable, foodCountLabel,
		func(score int32) { scoreLabel.SetText(fmt.Sprintf("Счет: %d", score)) },
		func(name string) { nameLabel.SetText(fmt.Sprintf("Имя: %v", name)) },
		func(role pb.NodeRole) { roleLabel.SetText(fmt.Sprintf("Роль: %v", role)) },
	)
}

// StartGameLoopForPlayer главный цикл игры для игрока
func StartGameLoopForPlayer(w fyne.Window, playerNode *player.Player, gameContent *fyne.Container,
	scoreTable *widget.Table, foodCountLabel *widget.Label, updateScore func(int32), updateName func(string), updateRole func(pb.NodeRole)) {

	rand.NewSource(time.Now().UnixNano())

	gameTicker = time.NewTicker(time.Millisecond * 60)

	isRunning = true
	playerNode.Node.RoleChangeChan = make(chan pb.NodeRole, 1) // Канал для сигналов смены роли

	// Обработка клавиш
	if playerNode.Node.Role == pb.NodeRole_NORMAL {
		w.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
			handleKeyInputForPlayer(e, playerNode)
		})
	}

	if playerNode.Node.State == nil {
		playerNode.Node.Mu.Lock()
		for playerNode.Node.State == nil {
			playerNode.Node.Cond.Wait()
		}
		playerNode.Node.Mu.Unlock()
	}

	go func() {
		for isRunning {
			select {
			case <-gameTicker.C:
				playerNode.Node.Mu.Lock()
				stateCopy := proto.Clone(playerNode.Node.State).(*pb.GameState)
				configCopy := proto.Clone(playerNode.Node.Config).(*pb.GameConfig)
				// Обновление счёта
				var playerScore int32
				for _, gamePlayer := range playerNode.Node.State.GetPlayers().GetPlayers() {
					if gamePlayer.GetId() == playerNode.Node.PlayerInfo.GetId() {
						playerScore = gamePlayer.GetScore()
						break
					}
				}
				updateScore(playerScore)
				updateName(playerNode.Node.PlayerInfo.GetName())
				updateRole(playerNode.Node.PlayerInfo.GetRole())
				renderGameState(gameContent, stateCopy, configCopy)
				updateInfoPanel(scoreTable, foodCountLabel, stateCopy)
				playerNode.Node.Mu.Unlock()
			case newRole := <-playerNode.Node.RoleChangeChan:
				log.Printf("Смена роли на %v", newRole)
				if newRole == pb.NodeRole_MASTER {
					// Завершаем текущий цикл игрока
					StopGameLoop()
					// Запускаем логику мастера
					go StartGameLoopForMaster(w, playerNode.Node, gameContent, scoreTable, foodCountLabel, updateScore, updateName, updateRole)
					return
				}
			}
		}
	}()
}

// handleKeyInputForPlayer обработка нажатий клавиш у игрока и отправка SteerMsg
func handleKeyInputForPlayer(e *fyne.KeyEvent, playerNode *player.Player) {
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

	playerNode.Node.Mu.Lock()
	defer playerNode.Node.Mu.Unlock()

	steerMsg := &pb.GameMessage{
		MsgSeq: proto.Int64(playerNode.Node.MsgSeq),
		Type: &pb.GameMessage_Steer{
			Steer: &pb.GameMessage_SteerMsg{
				Direction: newDirection.Enum(),
			},
		},
	}

	playerNode.Node.SendMessage(steerMsg, playerNode.MasterAddr)
}
