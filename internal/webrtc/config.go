package webrtc

// WebRTC configuration
// STUN/TURN server settings go here

var (
	// Add STUN and TURN servers
	StunServers = []string{
		"stun:stun.l.google.com:19302",
	}

	TurnServers = []map[string]interface{}{
		// Add TURN servers with credentials if needed
	}
)
