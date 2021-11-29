const webSocketUrl = location.href.replace("https://", "");

var microphoneState = true;
var videoState = true;
var localStream;
var ws;
var isStreaming = false;

const projectorContainerId = "id-projector-screen";
const remoteVideosContainerId = "id-remote-videos";
const videoElementLocalId = "id-video-el-local";
const videoElementLocalClass = "video-el-local";
const videoElementSiblingClass = "video-el-sibling";
const localVideoElement = document.getElementById(videoElementLocalId);
const projectorContainer = document.getElementById(projectorContainerId);
let projectorVideoElement;
// = getVideoElementInProjector();
setVideoElementInProjector();
const localVideoThumbnailContainer = document.getElementById("id-local-video-thumbnail-container");
const remoteVideosContainer = document.getElementById(remoteVideosContainerId);

const callDurationElement = document.getElementById("time-lapsed");

let callDurationObserver;
let callDurationObserverInSeconds = 0;

function initUpdateCallDuration() {
    callDurationObserverInSeconds++;
    updateCallDuratin();
}
function updateCallDuratin(duration) {
    var date = new Date(null);
    date.setSeconds(callDurationObserverInSeconds); // specify value for SECONDS here
    callDurationElement.innerHTML = date.toISOString().substr(11, 8);;
}
function clearCallDurationObserver() {
    clearInterval(callDurationObserver);
    callDurationObserverInSeconds = 0;
}


function setLocalStream(stream) {
    // console.log(stream);
    localStream = stream;
}


const microphoneStateElement = document.getElementById('microphone-state');
const microphoneStateElementIcon = microphoneStateElement.getElementsByTagName("i")[0];
microphoneStateElement.addEventListener("click", function(e) {
    e.preventDefault();
    microphoneState = (microphoneState) ? false : true;
    microphoneStateElementIcon.innerHTML = (microphoneState) ? "mic" : "mic_off";
    toggleAudio();
})

const cameraStateElement = document.getElementById('camera-state');
const cameraStateElementIcon = cameraStateElement.getElementsByTagName("i")[0];
cameraStateElement.addEventListener("click", function(e) {
    e.preventDefault();
    videoState = (videoState) ? false : true;
    // this.innerHTML = (videoState) ? "Block" : "Unblock";
    cameraStateElementIcon.innerHTML = (videoState) ? "videocam" : "videocam_off";
    toggleVideo();
})


const toggleMeetingElement = document.getElementById("toggle-meeting");
const toggleMeetingElementIcon = toggleMeetingElement.getElementsByTagName("i")[0];
toggleMeetingElement.addEventListener("click", function(e) {
    // connect(localStream);
    // console.log(ws);
    if (typeof ws === "undefined" || !isStreaming) {
        // this.innerHTML = " Leave Meeting ";
        toggleMeetingElementIcon.innerHTML = "phone_forwarded";
        toggleMeetingElement.classList.remove("indigo");
        toggleMeetingElement.classList.add("red");
        connect(localStream);

        callDurationObserver = setInterval(initUpdateCallDuration, 1000);
    } else {
        // this.innerHTML = " Join Meeting ";
        toggleMeetingElementIcon.innerHTML = "phone";
        toggleMeetingElement.classList.remove("red");
        toggleMeetingElement.classList.add("indigo");
        // trigger onclose before close socket is attempted
        ws.onclose = function () {}; // disable onclose handler first
        ws.close();
        leaveMeeting();
        clearCallDurationObserver();
    }
});


let logContainer = document.getElementById("logs");
navigator.mediaDevices.getUserMedia({ video: videoState, audio: microphoneState }).then(stream => {
    log("Camera and microphone are ready for use. Test your settings and press join meeting.");
    projectorVideoElement.srcObject = stream;
    setLocalStream(stream);

}).catch(window.alert)

function log(msg) {
    var newElement = document.createElement('div');
    // newElement.innerHTML = xmlhttp.responseText;
    newElement.innerHTML = msg;
    logContainer.appendChild(newElement);
}


function toggleAudio() {
    localStream.getAudioTracks()[0].enabled = !(localStream.getAudioTracks()[0].enabled);
}

function toggleVideo() {
    localStream.getVideoTracks()[0].enabled = !(localStream.getVideoTracks()[0].enabled);
}

function connect(stream) {

    let pc = new RTCPeerConnection()
    pc.ontrack = function (event) {
        if (event.track.kind === 'audio') {
            log("its audio");
            return
        }

        let el = document.createElement(event.track.kind)
        el.srcObject = event.streams[0]
        el.autoplay = true;
        el.controls = false;
        el.setAttribute("playsinline", true);
        addTrack(el);

        event.track.onmute = function(event) {
            el.play()
        }

        event.streams[0].onremovetrack = ({track}) => {
            // console.log(el)
            if (el.parentNode) {
                // console.log(track);
                removeTrack(el);
                // el.parentNode.removeChild(el);
                log(`Peer with ID ${track.id} left`);
            }
        }   
    }

    // document.getElementById('localVideo').srcObject = stream
    stream.getTracks().forEach(track => pc.addTrack(track, stream))
    // setLocalStream(stream);

    //let ws = new WebSocket("{{.}}")
    ws = new WebSocket(`wss://${webSocketUrl}/websocket`);




    pc.onicecandidate = e => {
        if (!e.candidate) {
            return
        }

        // console.log(e.candidate)

        ws.send(JSON.stringify({event: 'candidate', data: JSON.stringify(e.candidate)}))
    }

    ws.onclose = function(evt) {
        // window.alert("Websocket has closed")
        log("WebSocket has closed");
    }

    ws.onmessage = function(evt) {
        let msg = JSON.parse(evt.data)
        if (!msg) {
            return console.log('failed to parse msg')
        }

        // console.log(msg);

        switch (msg.event) {
            case 'offer':
                let offer = JSON.parse(msg.data)
                if (!offer) {
                    return console.log('failed to parse answer')
                }
                pc.setRemoteDescription(offer)
                pc.createAnswer().then(answer => {
                    pc.setLocalDescription(answer)
                    ws.send(JSON.stringify({event: 'answer', data: JSON.stringify(answer)}))
                })
                return

            case 'candidate':
                log("candidate");
                let candidate = JSON.parse(msg.data)
                if (!candidate) {
                    return console.log('failed to parse candidate')
                }

                pc.addIceCandidate(candidate)
                isStreaming = true;
        }
    }

    ws.onerror = function(evt) {
        log("Connection ERROR: " + evt.data);
        console.log("ERROR: " + evt.data)
    }
}


document.onclick = function(e) {

    if (e.target.classList.contains(videoElementSiblingClass) ) {
        // console.log(e.target.srcObject);
        // document.getElementById("remoteVideos").appendChild(e.target);
        e.target.classList.remove(videoElementSiblingClass);
        swichtVideoStreamToProjector(e.target);
    }
}


function swichtVideoStreamToProjector(videoElement) {
    setVideoElementInProjector();
    if (typeof projectorVideoElement === "undefined") return;
    if (projectorVideoElement.classList.contains(videoElementLocalClass)) {
        localVideoThumbnailContainer.appendChild(projectorVideoElement);
    } else {
        projectorVideoElement.classList.add(videoElementSiblingClass);
        remoteVideosContainer.appendChild(projectorVideoElement);
    }

    projectorContainer.appendChild(videoElement);
}


function removeTrack(videoElement) {
    // the track is running on the main projector
    if (videoElement.parentNode.id == projectorContainerId) {
        // check if there are more tracks in remote container
        let firstRemoteElement = getFirstTrackFromRemoteVideos();
        if (typeof firstRemoteElement !== "undefined") {
            swichtVideoStreamToProjector(firstRemoteElement);
        } else {
            // apparently there were no streams in remote container
            // hence project the local stream 
            swichtVideoStreamToProjector(getVideoElementInThumbnail());
        }
    }
    
    videoElement.parentNode.removeChild(videoElement);
}


function addTrack(videoElement) {
    videoElement.classList.add(videoElementSiblingClass);
    swichtVideoStreamToProjector(videoElement);
}



function leaveMeeting() {
    isStreaming = false;
    setVideoElementInProjector();

    if (!projectorVideoElement.classList.contains(videoElementLocalClass)) {
        projectorContainer.innerHTML = "";
        projectorContainer.appendChild(getVideoElementInThumbnail());
    }

    remoteVideosContainer.innerHTML = "";
    localVideoThumbnailContainer.innerHTML = "";
    
}


function setVideoElementInProjector() {
    projectorVideoElement = getVideoElementInProjector();
}


function getVideoElementInThumbnail() {
    return localVideoThumbnailContainer.getElementsByTagName('video')[0];
}

function getVideoElementInProjector() {
    return projectorContainer.getElementsByTagName('video')[0];
}


function getFirstTrackFromRemoteVideos() {
    return remoteVideosContainer.getElementsByTagName('video')[0];
}


