
// Full Screen
function toggleFullscreen() {
    if (!document.fullscreenElement) {
        document.documentElement.requestFullscreen();
    } else {
        if (document.exitFullscreen) {
            document.exitFullscreen();
        }
    }
}






//  Hats and Characters
hat = document.getElementById("hat-img");
character = document.getElementById("char-img");
var hat_counter = 1;
var character_counter = 1;

hat.style.backgroundImage = `url(/assets/hats/${hat_counter}.png)`;
character.style.backgroundImage = `url(/assets/characters/${character_counter}_idle.png)`;

function hats(modify, maxVal) {
    if (hat_counter+modify > maxVal || hat_counter+modify == 0) return
    hat_counter += modify;
    hat.style.backgroundImage = `url(/assets/hats/${hat_counter}.png)`;
}

function characters(modify, maxVal) {
    if (character_counter+modify > maxVal || character_counter+modify == 0) return
    character_counter += modify;
    character.style.backgroundImage = `url(/assets/characters/${character_counter}_idle.png)`;
}






let socket;
let playerName
// Game start
function togglePrompt(){
    // Check if player has name
    playerName = document.getElementById('nameInput').value
    if (playerName == "") return
    
    // Make a new websocket
    socket = new WebSocket("ws://192.168.100.29:80/ws");
    socket.addEventListener("open", (event) => {
        socket.send("NEW " + hat_counter + " " + character_counter + " " + playerName);
    });
    
    p = document.getElementById('prompt')
    toggleFullscreen()
    p.style.transition = `margin-top 2s ease-in-out`
    p.style.marginTop = `100vh`
    setTimeout(() => {
        p.style.transition = `unset`;
    }, 2000);
    
    unlockBall()
}

function unlockBall() {
    let dragging = false
    let ball = document.getElementById('ball')
    
    const circleCenterY = window.innerHeight*.50;
    const circleCenterX = window.innerWidth*.25 ;
    const circleRadius = window.innerHeight*.25;
    
    function onStart(event) {
        if (event.target.tagName.toLowerCase() !== 'button') {
            dragging = true
        }
    }

    function onDrag(event) {
        if (!dragging) return;
        let transX =  event.touches[0].clientX - window.innerWidth * 0.25;
        let transY = event.touches[0].clientY - window.innerHeight * 0.50;


        if (Math.sqrt(transX*transX+transY*transY)>window.innerHeight*.25) {
            let point = closestPointOnCircle(transX, transY, circleCenterX, circleCenterY, circleRadius);
            transX = point.x
            transY = point.y
        }

        ball.style.transform = `translateX(${transX}px) translateY(${transY}px)`;
        //socket.send("BAL "+ transX/circleRadius*100 + " " + transY/circleRadius*100)

    }

    function onStop(event) {
        ball.style.transform = `translateX(0) translateY(0)`
        ball.style.margin = `unset`
        ball.style.marginTop = circleCenterY
        ball.style.marginLeft = circleCenterX
        //socket.send("BAL 0.0 0.0")
    }


    function closestPointOnCircle(transX, transY, circleCenterX, circleCenterY, circleRadius) {
        const d = Math.sqrt(transX*transX + transY*transY);
        const ratio = circleRadius / d


        const closestPointX = transX*ratio;
        const closestPointY = transY*ratio;
    
        // Return the coordinates of the closest point on the circle
        return { x: closestPointX, y: closestPointY };
    }

    document.getElementById('ballCircle').addEventListener('touchstart', onStart);
    document.getElementById('ballCircle').addEventListener('touchmove', onDrag);
    document.getElementById('ballCircle').addEventListener('touchend', onStop);

    function getTranslateXY(element) {
        const style = window.getComputedStyle(element)
        const matrix = new DOMMatrixReadOnly(style.transform)
        return {
            translateX: matrix.m41,
            translateY: matrix.m42
        }
    }

    let bombsLeft = 0
    let health = 0
    socket.addEventListener('message', (event) => {
        let message = event.data
        if (message.substring(0, 3) == "BOM" && message.split("\\\\")[1] == playerName) {
            bombsLeft = message.split("\\\\")[2]
        }
        
        if (message.substring(0, 3) == "HEL" && message.split("\\\\")[1] == playerName) {
            health = message.split("\\\\")[2]
        }

        if (message.substring(0, 3) == "QUE") {
            let vals = message.split("\\\\")
            document.getElementById('question').textContent = vals[1]
            document.getElementById('response1').textContent = vals[2]
            document.getElementById('response2').textContent = vals[3]
            document.getElementById('response3').textContent = vals[4]

            document.getElementById('triviaBox').style.display = `unset`
        } 
    });

    requestAnimationFrame(update)
    function update() {
        if (!(socket.readyState === WebSocket.OPEN)) {
            console.log("Socket not ready")
            requestAnimationFrame(update)
            return
        }
        console.log("Socket ready")
        let transform = getTranslateXY(ball)
        socket.send("BAL "+ transform.translateX/circleRadius*100 + " " + transform.translateY/circleRadius*100)


        document.getElementById('bombCounter').textContent = bombsLeft
        document.getElementById('healthBar').style.width = `${health*6}px`

        requestAnimationFrame(update)
    }


}

function greenBTN() {
    socket.send("BTN GREEN")
}
function redBTN() {
    socket.send("BTN RED")
}

document.getElementById('response1').addEventListener("touchstart", () => {
    socket.send("RSP " + 1)
    document.getElementById('triviaBox').style.display = `none`
})
document.getElementById('response2').addEventListener("touchstart", () => {
    socket.send("RSP " + 2)
    document.getElementById('triviaBox').style.display = `none`
})
document.getElementById('response3').addEventListener("touchstart", () => {
    socket.send("RSP " + 3)
    document.getElementById('triviaBox').style.display = `none`
})

