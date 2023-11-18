package main

import (
	"encoding/json"
	"fmt"
	"image"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	_ "image/png"

	"github.com/faiface/pixel"
	"github.com/faiface/pixel/pixelgl"
	"github.com/faiface/pixel/text"
	"github.com/gorilla/websocket"
	"golang.org/x/image/colornames"
	"golang.org/x/image/font/basicfont"
)

var wd string
var maxHats int
var maxChars int

const windowX = 1920
const windowY = 1080
const bottomFloor = 100
const gravity = 1000
const globalTerminalVelocityX = 1000
const globalTerminalVelocityY = 1000
const globalJumpPower = 400
const gloablSpeed = 35.
const constantXLoss = 5.
const lavaDamage = 100
const explosionFuse = time.Second * 1
const explosionDamage = 0
const explosionPower = 10000
const explosionSpread = 1.
const explosionDecay = .01
const minBombsLeft = 3
const correctAnswerPoints = 1000.
const maxLevelPoints = 10000.
const nonCompletionPenalty = 1000
const podiumDisplayTime = time.Second * 5

var gameStarted = false

type player struct {
	playerName       string
	hatID            int
	characterID      int
	animation        string
	wearingHat       bool
	IP               string
	winner           bool
	score            float64
	position         struct{ X, Y float64 }
	acceleration     struct{ X, Y float64 }
	terminalVelocity struct{ X, Y float64 }
	grounded         bool
	jumpPower        float64
	speed            float64
	bombsLeft        int
	health           float64
	finishDuration   time.Duration
	exploding        bool
	explosionFuse    time.Time
	claimedBombs     []struct{ X, Y int }
}

type goober struct {
	idle          pixel.Sprite
	walking_right pixel.Sprite
	walking_left  pixel.Sprite
	falling       pixel.Sprite
	exploding     pixel.Sprite
}

type block struct {
	blockType string
}

type particle struct {
	created  time.Time
	lifespan time.Duration
	position pixel.Vec
	sprite   pixel.Sprite
}

type question struct {
	Question string `json:"Question"`
	Corect   string `json:"Correct"`
	Alt1     string `json:"Alt1"`
	Alt2     string `json:"Alt2"`
}

var players []player
var blockGrid [][]block
var particles []particle
var questions []question

func _init() {
	//* Get wd
	var err error
	wd, err = os.Getwd()
	if err != nil {
		panic(err)
	}

	//* Get maxElems
	characters, err := os.ReadDir(path.Join(wd, "\\assets\\characters"))
	if err != nil {
		panic(err)
	}
	hats, err := os.ReadDir(path.Join(wd, "\\assets\\hats"))
	if err != nil {
		panic(err)
	}
	maxChars = len(characters)
	maxHats = len(hats)

	//* Build blockgrid
	for i := 0; i < windowX/50+1; i++ {
		var row []block
		for j := 0; j < windowY/50; j++ {
			row = append(row, block{blockType: ""})
		}
		blockGrid = append(blockGrid, row)
	}

	//* Get questions
	rawQuestions, err := os.ReadFile(path.Join(wd, "questions.json"))
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(rawQuestions, &questions)
	if err != nil {
		panic(err)
	}
}

func readHTML(name string) string {
	f, err := os.ReadFile(path.Join(wd, "\\static\\", name+".html"))
	if err != nil {
		fmt.Println("Failed to read file ", name)
	}
	return string(f)
}

func main() {
	_init()

	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))
	http.Handle("/scripts/", http.StripPrefix("/scripts/", http.FileServer(http.Dir("scripts"))))
	http.Handle("/styles/", http.StripPrefix("/styles/", http.FileServer(http.Dir("styles"))))

	http.HandleFunc("/", handleControls)
	http.HandleFunc("/ws", handleWebSocket)

	go func() {
		err := http.ListenAndServe(":80", nil)
		if err != nil {
			fmt.Println("All the controllers have been disconected!")
		}
	}()

	fmt.Println("Hello, world!")

	pixelgl.Run(run)
}

func handleControls(w http.ResponseWriter, r *http.Request) {
	html := readHTML("root")

	html = strings.ReplaceAll(html, "'{{ MaxHats }}'", fmt.Sprint(maxHats))
	html = strings.ReplaceAll(html, "'{{ MaxChars }}'", fmt.Sprint(maxChars))

	fmt.Fprint(w, html)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow any origin
	},
}

func findPlayerByIP(ip string) int {
	var ID = -1

	for i := 0; i < len(players); i++ {
		if players[i].IP == ip {
			ID = i
			break
		}
	}

	return ID
}

func dist(x float64, y float64, a float64, b float64) float64 {
	return math.Sqrt((x-a)*(x-a) + (y-b)*(y-b))
}

var conn *websocket.Conn

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	var err error
	conn, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	fmt.Println("Client connected")

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			fmt.Println(err)
			return
		}

		// Add new players
		if string(msg[:3]) == "NEW" {
			thisHatID, err := strconv.Atoi(strings.Split(string(msg), " ")[1])
			if err != nil {
				fmt.Println("Failed to register player.")
				continue
			}
			thisCharacterID, err := strconv.Atoi(strings.Split(string(msg), " ")[2])
			if err != nil {
				fmt.Println("Failed to register player.")
				continue
			}
			players = append(players, player{
				hatID:        thisHatID,
				characterID:  thisCharacterID,
				wearingHat:   true,
				playerName:   strings.Split(string(msg), " ")[3],
				animation:    "idle",
				IP:           r.RemoteAddr,
				winner:       false,
				score:        100000,
				position:     struct{ X, Y float64 }{0.0, windowY},
				acceleration: struct{ X, Y float64 }{0.0, -100.0},
				terminalVelocity: struct {
					X float64
					Y float64
				}{globalTerminalVelocityX, globalTerminalVelocityY},
				grounded:     true,
				jumpPower:    globalJumpPower,
				speed:        gloablSpeed,
				bombsLeft:    minBombsLeft,
				health:       100,
				claimedBombs: []struct{ X, Y int }{},
			})
			fmt.Println("New player: ", players[len(players)-1])
			continue
		}

		// Check jumps
		playerID := findPlayerByIP(r.RemoteAddr)
		if playerID == -1 {
			continue
		}
		if string(msg) == "BTN GREEN" && gameStarted && players[playerID].grounded {
			players[playerID].acceleration.Y += players[playerID].jumpPower
			players[playerID].grounded = false
		}

		// Check movements
		if string(msg[:3]) == "BAL" && gameStarted {
			ballX, err := strconv.ParseFloat(strings.Split(string(msg), " ")[1], 64)
			if err != nil {
				fmt.Println("Player submited invalid value for BAL")
				continue
			}
			players[playerID].acceleration.X += players[playerID].speed * deltaTime * ballX

		}

		if string(msg) == "BTN RED" && gameStarted && players[playerID].bombsLeft > 0 && !players[playerID].exploding {
			players[playerID].exploding = true
			players[playerID].explosionFuse = time.Now()
			players[playerID].wearingHat = false

			// Remove bombs from inventory
			players[playerID].bombsLeft -= 1
		}

		if string(msg[:3]) == "RSP" && gameStarted {
			if string(msg[4:]) == triviaAnswer {
				players[playerID].bombsLeft += 1
				players[playerID].score += correctAnswerPoints
			}
		}
	}
}

func notifyController(t time.Duration) {
	for {
		for i := range players {
			message := fmt.Sprintf("BOM\\\\%s\\\\%d", players[i].playerName, players[i].bombsLeft)
			conn.WriteMessage(websocket.TextMessage, []byte(message))

			message = fmt.Sprintf("HEL\\\\%s\\\\%f", players[i].playerName, players[i].health)
			conn.WriteMessage(websocket.TextMessage, []byte(message))

			time.Sleep(t)
		}
	}
}

func loadPicture(path string) (pixel.Picture, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	return pixel.PictureDataFromImage(img), nil
}

func getPrivateIP() (string, error) {
	toReturn := ""
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range interfaces {
		// Skip loopback and non-up interfaces
		if (iface.Flags&net.FlagLoopback == 0) && (iface.Flags&net.FlagUp != 0) {
			addrs, err := iface.Addrs()
			if err != nil {
				return "", err
			}

			for _, addr := range addrs {
				ip, _, err := net.ParseCIDR(addr.String())
				if err != nil {
					return "", err
				}

				// Check if the IP address is a private IPv4 address
				if ip.To4() != nil && (ip.IsLoopback() || ip.IsPrivate()) {
					toReturn = ip.String()
				}
			}
		}
	}
	if toReturn == "" {
		return "", fmt.Errorf("no private ip address found")
	} else {
		return toReturn, nil
	}
}

func gravityHandler(deltaTime float64) {
	for i, val := range players {
		gridPositionY := int(math.Floor((val.position.Y - 25) / 50))
		gridPositionX := int(math.Floor((val.position.X) / 50))

		// Apply acceleration
		cond := false
		if (gridPositionX > -1 && gridPositionX+1 < len(blockGrid)) && (gridPositionY > -1 && gridPositionY < len(blockGrid[0])) {
			cond = blockGrid[gridPositionX][gridPositionY].blockType != "" || blockGrid[gridPositionX+1][gridPositionY].blockType != ""
		}
		cond2 := false
		if (gridPositionX > -1 && gridPositionX < len(blockGrid)) && (gridPositionY > -1 && gridPositionY < len(blockGrid[0])) {
			if blockGrid[gridPositionX][gridPositionY].blockType == "lava" {
				cond2 = true
			}
		}
		cond3 := false
		if (gridPositionX > -1 && gridPositionX < len(blockGrid)) && (gridPositionY > -1 && gridPositionY < len(blockGrid[0])) {
			if blockGrid[gridPositionX][gridPositionY].blockType == "ability" {
				claimed := false
				for _, val := range players[i].claimedBombs {
					if val.X == gridPositionX && val.Y == gridPositionY {
						claimed = true
						break
					}
				}
				if claimed {
					continue
				}

				cond3 = true
			}
		}
		cond4 := false
		if (gridPositionX > -1 && gridPositionX < len(blockGrid)) && (gridPositionY > -1 && gridPositionY < len(blockGrid[0])) {
			if blockGrid[gridPositionX][gridPositionY].blockType == "finish" {
				cond4 = true
			}
		}

		if val.position.Y-bottomFloor < 50 || cond {

			// Stop at the bottom floor
			if players[i].acceleration.Y < 0 {
				players[i].acceleration.Y -= players[i].acceleration.Y
			}
			players[i].grounded = true
		}
		if val.position.Y < bottomFloor || cond2 {
			// Check if outside of map
			players[i].health -= lavaDamage * deltaTime
			if players[i].health <= 0 {
				players[i].finishDuration = time.Since(currentLevelStartTime)
			}

		} else if cond3 {
			players[i].bombsLeft += 1
			players[i].claimedBombs = append(players[i].claimedBombs, struct {
				X int
				Y int
			}{gridPositionX, gridPositionY})
		} else if cond4 {
			players[i].winner = true
			players[i].health = 0
			players[i].finishDuration = time.Since(currentLevelStartTime)
		}
		if val.position.Y > bottomFloor && !(val.position.Y-bottomFloor < 50 || cond) {
			// Add gravitational acceleration
			players[i].acceleration.Y -= deltaTime * gravity

		}
	}
}

func movementHandler(deltaTime float64) {
	for i, val := range players {
		gridPositionY := int(math.Round((val.position.Y) / 50))
		gridPositionX := int(math.Round((val.position.X - 25) / 50))

		// Check if everything is ok
		if val.acceleration.Y > val.terminalVelocity.Y {
			players[i].acceleration.Y = val.terminalVelocity.Y
		}
		if val.acceleration.X > val.terminalVelocity.X {
			players[i].acceleration.X = val.terminalVelocity.X
		}
		if val.acceleration.X < -val.terminalVelocity.X {
			players[i].acceleration.X = -val.terminalVelocity.X
		}
		if val.acceleration.Y < -val.terminalVelocity.Y {
			players[i].acceleration.Y = -val.terminalVelocity.Y
		}

		// Calculate movements
		changedX := float64(deltaTime) * players[i].acceleration.X
		changedY := float64(deltaTime) * players[i].acceleration.Y

		// Apply movements
		players[i].acceleration.X -= changedX
		players[i].acceleration.Y -= changedY
		// Apply some resistance
		if changedX == 0 {
			continue
		}
		players[i].acceleration.X -= (changedX / math.Abs(changedX)) * constantXLoss
		// Stop at ceilings
		if (gridPositionX > -1 && gridPositionX < len(blockGrid)) && (gridPositionY > -1 && gridPositionY < len(blockGrid[0])) {
			if blockGrid[gridPositionX][gridPositionY].blockType != "" {
				changedY -= math.Abs(changedY)
			}
		}
		players[i].position.Y += changedY

		// Stop at walls
		if (gridPositionX > -1 && gridPositionX+1 < len(blockGrid)) && (gridPositionY > -1 && gridPositionY+1 < len(blockGrid[0])) {
			if (changedX < 0 && blockGrid[gridPositionX][gridPositionY].blockType != "") || (changedX > 0 && blockGrid[gridPositionX+1][gridPositionY].blockType != "") {
				changedX -= changedX
			}
		}
		players[i].position.X += changedX
	}
}

func basicAnimator() {
	for {
		if !gameStarted {
			continue
		}
		for i := range players {
			if players[i].exploding {
				players[i].animation = "exploding"
			} else if players[i].acceleration.Y < 0 {
				players[i].animation = "falling"
			} else if math.Abs(players[i].acceleration.X) > 10 {
				if players[i].acceleration.X > 0 {
					players[i].animation = "walking-right"
				} else {
					players[i].animation = "walking-left"
				}
			} else {
				players[i].animation = "idle"
			}
		}
	}
}

func explosionManager() {
	thisIMG, err := loadPicture(path.Join(wd, "/assets/particles/explosion.png"))
	if err != nil {
		panic(err)
	}
	explosion := pixel.NewSprite(thisIMG, thisIMG.Bounds())

	for {
		time.Sleep(time.Millisecond * 100)
		if !gameStarted {
			continue
		}
		for i, val := range players {
			if !val.exploding {
				continue
			}
			if time.Since(val.explosionFuse) < explosionFuse {
				continue
			}

			// Make player back
			players[i].exploding = false
			players[i].wearingHat = true

			// Schedule some particles
			particles = append(particles, particle{
				created:  time.Now(),
				lifespan: time.Second * 1,
				position: val.position,
				sprite:   *explosion,
			})

			// Affect players
			go func(val player) {
				for j, vall := range players {
					if vall.IP == val.IP {
						continue
					}
					d := dist(vall.position.X, vall.position.Y, val.position.X, val.position.Y)
					dx := math.Abs(vall.position.X - val.position.X)
					dy := math.Abs(vall.position.Y - val.position.Y)
					pow := explosionPower * math.Exp(-explosionDecay*d) * explosionSpread
					ratio := d / (d + pow)
					powy := dy/ratio + (vall.position.Y-val.position.Y)/dy*val.position.Y
					powx := dx/ratio + (vall.position.X-val.position.X)/dx*val.position.X

					players[j].acceleration.X += powx
					players[j].acceleration.Y += powy

					players[j].health -= pow * explosionDamage

				}
			}(val)
		}
	}
}

func askPlayers() int {
	q := questions[rand.Intn(len(questions))]
	var r1, r2, r3 string
	var toReturn int
	switch rand.Intn(6) {
	case 0:
		r1 = q.Alt1
		r2 = q.Alt2
		r3 = q.Corect
		toReturn = 3
	case 1:
		r1 = q.Alt1
		r2 = q.Corect
		r3 = q.Alt2
		toReturn = 2
	case 2:
		r1 = q.Alt2
		r2 = q.Alt1
		r3 = q.Corect
		toReturn = 3
	case 3:
		r1 = q.Alt2
		r2 = q.Corect
		r3 = q.Alt1
		toReturn = 2
	case 4:
		r1 = q.Corect
		r2 = q.Alt1
		r3 = q.Alt2
		toReturn = 1
	case 5:
		r1 = q.Corect
		r2 = q.Alt2
		r3 = q.Alt1
		toReturn = 1
	}

	message := fmt.Sprintf("QUE\\\\%s\\\\%s\\\\%s\\\\%s", q.Question, r1, r2, r3)
	conn.WriteMessage(websocket.TextMessage, []byte(message))

	return toReturn
}

func placeAllPlayers(x, y float64) {
	for i := range players {
		players[i].position = struct {
			X float64
			Y float64
		}{x, y}
		players[i].acceleration = struct {
			X float64
			Y float64
		}{0, 0}
	}
}

func healAllPlayers() {
	for i := range players {
		players[i].health = 100
		players[i].claimedBombs = []struct {
			X int
			Y int
		}{}
	}
}

func clearBlockGrid() {
	for x := 0; x < len(blockGrid); x++ {
		for y := 0; y < len(blockGrid[0]); y++ {
			blockGrid[x][y] = block{}
		}
	}
}

func calculateLevelScore(t time.Duration) {
	for i := range players {
		if players[i].winner {
			players[i].score += (1 - float64(players[i].finishDuration.Milliseconds())/float64(t.Milliseconds())) * maxLevelPoints
		} else {
			players[i].score -= nonCompletionPenalty
		}
		players[i].winner = false
		fmt.Println(players[i].score)
	}

	timeAtPodiumAppeared = time.Now()
	showPodium = true
}

var showPodium = false
var timeAtPodiumAppeared = time.Now()
var currentLevelStartTime = time.Now()
var triviaAnswer string
var deltaTime float64

func run() {
	go basicAnimator()
	go explosionManager()
	go notifyController(time.Millisecond * 500)
	//* Init window
	cfg := pixelgl.WindowConfig{
		Title:     "Goobers!",
		Bounds:    pixel.R(0, 0, float64(windowX), float64(windowY)),
		VSync:     true,
		Maximized: true,
	}
	win, err := pixelgl.NewWindow(cfg)
	if err != nil {
		panic(err)
	}

	//* Basic sprites loading
	// Vignete
	pic, err := loadPicture(path.Join(wd, "\\assets\\vignete.png"))
	if err != nil {
		panic(err)
	}
	vignete := pixel.NewSprite(pic, pic.Bounds())
	// Backgrounds
	backgroundsDir, err := os.ReadDir(path.Join(wd, "/assets/backgrounds"))
	if err != nil {
		panic(err)
	}
	var backgrounds []pixel.Sprite
	for i := 0; i < len(backgroundsDir); i++ {
		thisIMG, err := loadPicture(path.Join(wd, "/assets/backgrounds", fmt.Sprint(i)+".png"))
		if err != nil {
			fmt.Println("Failed to load background: ", path.Join(wd, "/assets/backgrounds", fmt.Sprint(i)+".png"))
		}

		backgrounds = append(backgrounds, *pixel.NewSprite(thisIMG, thisIMG.Bounds()))
	}
	// Floor
	var floor pixel.Sprite
	if true {
		thisIMG, err := loadPicture(path.Join(wd, "/assets/blocks/floor.png"))
		if err != nil {
			panic(err)
		}
		floor = *pixel.NewSprite(thisIMG, thisIMG.Bounds())
	}
	// Basic block
	var basicBlock pixel.Sprite
	if true {
		thisIMG, err := loadPicture(path.Join(wd, "/assets/blocks/block.png"))
		if err != nil {
			panic(err)
		}
		basicBlock = *pixel.NewSprite(thisIMG, thisIMG.Bounds())
	}
	// Lava block
	var lavaBlock pixel.Sprite
	if true {
		thisIMG, err := loadPicture(path.Join(wd, "/assets/blocks/lava.png"))
		if err != nil {
			panic(err)
		}
		lavaBlock = *pixel.NewSprite(thisIMG, thisIMG.Bounds())
	}
	// Ability block
	var abilityBlock pixel.Sprite
	if true {
		thisIMG, err := loadPicture(path.Join(wd, "/assets/blocks/ability.png"))
		if err != nil {
			panic(err)
		}
		abilityBlock = *pixel.NewSprite(thisIMG, thisIMG.Bounds())
	}
	// Finish block
	var finishBlock pixel.Sprite
	if true {
		thisIMG, err := loadPicture(path.Join(wd, "/assets/blocks/finish.png"))
		if err != nil {
			panic(err)
		}
		finishBlock = *pixel.NewSprite(thisIMG, thisIMG.Bounds())
	}
	// Bar
	statusBar, err := loadPicture(path.Join(wd, "/assets/progress_bar.png"))
	if err != nil {
		panic(err)
	}
	// Podium
	var podium pixel.Sprite
	if true {
		thisIMG, err := loadPicture(path.Join(wd, "/assets/podium.png"))
		if err != nil {
			panic(err)
		}
		podium = *pixel.NewSprite(thisIMG, thisIMG.Bounds())
	}

	//* Prepare story
	storyDir, err := os.ReadDir(path.Join(wd, "\\assets\\story"))
	if err != nil {
		panic(err)
	}
	storyPages := len(storyDir)
	storyPage := 0
	storyTimeout := time.Now()
	//previousTime := time.Now()

	//* Prepare menu
	inMenu := true
	basicAtlas := text.NewAtlas(basicfont.Face7x13, text.ASCII)
	// Get IP
	privateIP, err := getPrivateIP()
	if err != nil {
		panic(err)
	}
	IPtext := text.New(pixel.V(float64(windowX)*75/100, float64(windowY)*10/100), basicAtlas)
	IPtext.Color = colornames.Black
	fmt.Fprintln(IPtext, "Invite Link:")
	IPtext.Color = colornames.Blue
	fmt.Fprintln(IPtext, privateIP)
	// Get text to start game
	pressToStartText := text.New(pixel.V(windowX*35/100, windowY*50/100), basicAtlas)
	pressToStartText.Color = colornames.Red
	pressToStartTextTimeout := time.Now()
	pressToStartTextDraw := false
	fmt.Fprintln(pressToStartText, "Press 'ENTER' to start!")
	// Get title
	titleIMG, err := loadPicture(path.Join(wd, "/assets/title.png"))
	if err != nil {
		panic(err)
	}
	titleSprite := pixel.NewSprite(titleIMG, titleIMG.Bounds())
	// Get background
	menuBackgroundID := rand.Intn(len(backgrounds))

	//* Load goobers
	goobersDir, err := os.ReadDir(path.Join(wd, "/assets/characters"))
	if err != nil {
		panic(err)
	}
	var goobers []goober
	for i := 0; i < len(goobersDir)/4; i++ {
		// IDLE
		thisIMG, err := loadPicture(path.Join(wd, "/assets/characters", fmt.Sprint(i+1)+"_idle.png"))
		if err != nil {
			panic(err)
		}
		this := goober{
			idle: *pixel.NewSprite(thisIMG, thisIMG.Bounds()),
		}
		// WALKING-RIGHT
		thisIMG, err = loadPicture(path.Join(wd, "/assets/characters", fmt.Sprint(i+1)+"_walking-right.png"))
		if err != nil {
			panic(err)
		}
		this.walking_right = *pixel.NewSprite(thisIMG, thisIMG.Bounds())
		// WALKING-LEFT
		thisIMG, err = loadPicture(path.Join(wd, "/assets/characters", fmt.Sprint(i+1)+"_walking-left.png"))
		if err != nil {
			panic(err)
		}
		this.walking_left = *pixel.NewSprite(thisIMG, thisIMG.Bounds())
		// FALLING
		thisIMG, err = loadPicture(path.Join(wd, "/assets/characters", fmt.Sprint(i+1)+"_falling.png"))
		if err != nil {
			panic(err)
		}
		this.falling = *pixel.NewSprite(thisIMG, thisIMG.Bounds())
		// EXPLODING
		thisIMG, err = loadPicture(path.Join(wd, "/assets/characters", fmt.Sprint(i+1)+"_exploding.png"))
		if err != nil {
			panic(err)
		}
		this.exploding = *pixel.NewSprite(thisIMG, thisIMG.Bounds())
		goobers = append(goobers, this)
	}

	//* Load hats
	hatsDir, err := os.ReadDir(path.Join(wd, "/assets/hats"))
	if err != nil {
		panic(err)
	}
	var hats []pixel.Sprite
	for i := range hatsDir {
		thisIMG, err := loadPicture(path.Join(wd, "/assets/hats", fmt.Sprint(i+1)+".png"))
		if err != nil {
			panic(err)
		}
		hats = append(hats, *pixel.NewSprite(thisIMG, thisIMG.Bounds()))
	}

	//*Level counter
	var currentLevelID = 0
	var levelDuration = time.Millisecond // preinit at a small number
	var showProgressBar = true

	//* Deltatime
	lastTime := time.Now()
	for !win.Closed() {
		deltaTime = time.Since(lastTime).Seconds()
		lastTime = time.Now()

		//* Clear
		win.Clear(colornames.Skyblue)

		//* Read story
		if !(storyPage >= storyPages) {

			pic, err := loadPicture(path.Join(wd, "\\assets\\story", fmt.Sprint(storyPage)+".png"))
			if err != nil {
				panic(err)
			}
			thisStoryPage := pixel.NewSprite(pic, pic.Bounds())
			thisStoryPage.Draw(win, pixel.IM.Moved(win.Bounds().Center()))

			if time.Since(storyTimeout) >= time.Second*2 {
				storyTimeout = time.Now()
				storyPage++
			}
			vignete.Draw(win, pixel.IM.Moved(win.Bounds().Center()).Scaled(win.Bounds().Center(), 1))
			// Key to skip story
			if win.JustPressed(pixelgl.KeyKPEnter) || win.JustPressed(pixelgl.KeyEnter) {
				if !(storyPage >= storyPages) {
					storyPage = storyPages
					continue
				}
			}
			win.Update()
			continue
		}

		//* Open menu
		if inMenu {
			// Put background
			backgrounds[menuBackgroundID].Draw(win, pixel.IM.Moved(win.Bounds().Center()))

			// Get players
			numOfPlayers := text.New(pixel.V(float64(windowX)*2.5/100, float64(windowY)*10/100), basicAtlas)
			numOfPlayers.Color = colornames.Black
			fmt.Fprintln(numOfPlayers, "Number of players:")
			numOfPlayers.Color = colornames.Blue
			fmt.Fprintln(numOfPlayers, len(players))

			titleSprite.Draw(win, pixel.IM.Moved(pixel.V(win.Bounds().Center().X, windowY*85/100)))
			IPtext.Draw(win, pixel.IM.Scaled(IPtext.Orig, 4))
			numOfPlayers.Draw(win, pixel.IM.Scaled(numOfPlayers.Orig, 4))

			if time.Since(pressToStartTextTimeout) >= time.Millisecond*1000 {
				pressToStartTextTimeout = time.Now()
				pressToStartTextDraw = !pressToStartTextDraw
			}
			if pressToStartTextDraw {
				pressToStartText.Draw(win, pixel.IM.Scaled(pressToStartText.Orig, 3))
			}

			if (win.JustPressed(pixelgl.KeyEnter) || win.JustPressed(pixelgl.KeyKPEnter)) && len(players) > 0 {
				inMenu = false
			}

			win.Update()
			continue
		}
		//! Render loop

		//* Load level
		if time.Since(currentLevelStartTime) >= levelDuration {
			currentLevelID++
			currentLevelStartTime = time.Now()
			triviaAnswer = fmt.Sprint(askPlayers())

			if currentLevelID == 1 {
				levelDuration = level1()
			} else if currentLevelID == 2 {
				calculateLevelScore(levelDuration)
				levelDuration = level2()
			} else {
				calculateLevelScore(levelDuration)
				levelDuration = time.Hour * 24
			}

		}

		gameStarted = true
		//* Render floor
		floor.Draw(win, pixel.IM.Moved(pixel.V(win.Bounds().Center().X, 50)))

		//* Render blocks
		for x := range blockGrid {
			for y := range blockGrid[0] {
				var choseBlock pixel.Sprite

				switch blockGrid[x][y].blockType {
				case "basic":
					choseBlock = basicBlock
				case "lava":
					choseBlock = lavaBlock
				case "ability":
					choseBlock = abilityBlock
				case "finish":
					choseBlock = finishBlock
				default:
					continue
				}

				choseBlock.Draw(win, pixel.IM.Moved(pixel.V(float64(x*50), float64(y*50+25))))
			}
		}

		//* Render players
		for _, val := range players {
			if val.health <= 0 {
				continue
			}
			var toDraw pixel.Sprite
			switch val.animation {
			case "idle":
				toDraw = goobers[val.characterID-1].idle
			case "walking-right":
				toDraw = goobers[val.characterID-1].walking_right
			case "walking-left":
				toDraw = goobers[val.characterID-1].walking_left
			case "falling":
				toDraw = goobers[val.characterID-1].falling
			case "exploding":
				toDraw = goobers[val.characterID-1].exploding
			default:
			}

			toDraw.Draw(win, pixel.IM.Moved(pixel.V(float64(val.position.X), float64(val.position.Y))))

		}

		//* Render hats
		for _, val := range players {
			if val.health <= 0 || !val.wearingHat {
				continue
			}
			hats[val.hatID-1].Draw(win, pixel.IM.Moved(pixel.V(float64(val.position.X), float64(val.position.Y+30))))
		}

		//* Render time
		if showProgressBar {
			// Show bar
			levelPercent := float64(time.Since(currentLevelStartTime).Milliseconds()) / float64(levelDuration.Milliseconds()) * 100.0
			pixel.NewSprite(statusBar, statusBar.Bounds()).Draw(win, pixel.IM.Moved(pixel.V(win.Bounds().Center().X, windowY*90/100)).ScaledXY(win.Bounds().Center(), pixel.V(1-levelPercent/100, 1)))
			// Show text
			timeLeft := text.New(pixel.V(0, 0), basicAtlas)
			timeLeft.Color = colornames.Black
			fmt.Fprintf(timeLeft, "Ending in: %s", (-time.Since(currentLevelStartTime) + levelDuration).Round(time.Millisecond*100).String())
			timeLeft.Draw(win, pixel.IM.Moved(win.Bounds().Center()).Scaled(win.Bounds().Center(), 4).Moved(pixel.V(-statusBar.Bounds().W()/4, windowY*40/100)))

		}

		//* Render particles
		for _, val := range particles {
			if time.Since(val.created) > val.lifespan {
				continue
			}
			val.sprite.Draw(win, pixel.IM.Moved(val.position))
		}

		//* Render podium
		if showPodium {
			if time.Since(timeAtPodiumAppeared) >= podiumDisplayTime {
				showPodium = false
			}

			// Find most influential players
			top1 := 0
			top2 := 0
			top3 := 0
			for i := range players {
				if players[i].score > players[top1].score {
					top3 = top2
					top2 = top1
					top1 = i
				} else if players[i].score > players[top2].score {
					top3 = top2
					top2 = i
				} else if players[i].score > players[top3].score {
					top3 = i
				}
			}
			if len(players) < 2 {
				top2 = top1
				top3 = top2
			} else if len(players) < 3 {
				top3 = top2
			}

			// Draw podium
			podium.Draw(win, pixel.IM.Moved(win.Bounds().Center()))

			// Draw players
			goobers[players[top3].characterID-1].idle.Draw(win, pixel.IM.Scaled(pixel.V(0, 0), 4).Moved(pixel.V(300, 500)))
			goobers[players[top1].characterID-1].idle.Draw(win, pixel.IM.Scaled(pixel.V(0, 0), 4).Moved(pixel.V(960, 500)))
			goobers[players[top2].characterID-1].idle.Draw(win, pixel.IM.Scaled(pixel.V(0, 0), 4).Moved(pixel.V(1620, 500)))

			// Draw scores
			s3 := text.New(pixel.V(0, 0), basicAtlas)
			s3.Color = colornames.Orange
			fmt.Fprintln(s3, players[top3].score)
			s3.Draw(win, pixel.IM.Scaled(pixel.V(0, 0), 4).Moved(pixel.V(300-s3.Bounds().W()*2, 700)))

			s2 := text.New(pixel.V(0, 0), basicAtlas)
			s2.Color = colornames.Orange
			fmt.Fprintln(s2, players[top2].score)
			s2.Draw(win, pixel.IM.Scaled(pixel.V(0, 0), 4).Moved(pixel.V(1620-s2.Bounds().W()*2, 700)))

			s1 := text.New(pixel.V(0, 0), basicAtlas)
			s1.Color = colornames.Orange
			fmt.Fprintln(s1, players[top1].score)
			s1.Draw(win, pixel.IM.Scaled(pixel.V(0, 0), 4).Moved(pixel.V(960-s1.Bounds().W()*2, 900)))

			// Draw player names
			n3 := text.New(pixel.V(0, 0), basicAtlas)
			n3.Color = colornames.White
			fmt.Fprintln(n3, players[top3].playerName)
			n3.Draw(win, pixel.IM.Scaled(pixel.V(0, 0), 4).Moved(pixel.V(300-n3.Bounds().W()*2, 325)))

			n2 := text.New(pixel.V(0, 0), basicAtlas)
			n2.Color = colornames.White
			fmt.Fprintln(n2, players[top2].playerName)
			n2.Draw(win, pixel.IM.Scaled(pixel.V(0, 0), 4).Moved(pixel.V(1620-n2.Bounds().W()*2, 325)))

			n1 := text.New(pixel.V(0, 0), basicAtlas)
			n1.Color = colornames.White
			fmt.Fprintln(n1, players[top1].playerName)
			n1.Draw(win, pixel.IM.Scaled(pixel.V(0, 0), 4).Moved(pixel.V(960-n1.Bounds().W()*2, 325)))
		}

		gravityHandler(deltaTime)
		movementHandler(deltaTime)
		//! KEYS

		win.Update()

	}
}

func level1() time.Duration {
	placeAllPlayers(100, 175)
	healAllPlayers()
	clearBlockGrid()

	blockGrid[0][4].blockType = "basic"
	blockGrid[1][4].blockType = "basic"
	blockGrid[2][4].blockType = "basic"
	blockGrid[3][4].blockType = "basic"
	blockGrid[3][5].blockType = "basic"
	blockGrid[3][6].blockType = "basic"
	blockGrid[3][7].blockType = "basic"
	blockGrid[3][8].blockType = "ability"
	blockGrid[4][8].blockType = "finish"
	blockGrid[5][8].blockType = "finish"
	blockGrid[6][8].blockType = "lava"

	return time.Second * 20
}

func level2() time.Duration {
	placeAllPlayers(100, 275)
	healAllPlayers()
	clearBlockGrid()

	blockGrid[0][4].blockType = "lava"
	blockGrid[1][4].blockType = "lava"
	blockGrid[2][4].blockType = "lava"
	blockGrid[3][4].blockType = "lava"
	blockGrid[3][5].blockType = "basic"
	blockGrid[3][6].blockType = "basic"
	blockGrid[3][7].blockType = "basic"
	blockGrid[3][8].blockType = "ability"
	blockGrid[4][8].blockType = "finish"
	blockGrid[5][8].blockType = "finish"
	blockGrid[6][8].blockType = "lava"

	return time.Second * 2
}
