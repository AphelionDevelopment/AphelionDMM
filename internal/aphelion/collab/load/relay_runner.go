package load

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/relayclient"
)

type RelayBundle struct {
	RelayURL        string                   `json:"relay_url"`
	RoomID          string                   `json:"room_id"`
	OwnerPrivateKey string                   `json:"owner_private_key"`
	GroupKey        string                   `json:"group_key"`
	Participants    []RelayBundleParticipant `json:"participants"`
}

type RelayBundleParticipant struct {
	PrivateKey string          `json:"private_key"`
	Admission  string          `json:"admission"`
	Role       protocolv2.Role `json:"role"`
}

type RelayResult struct {
	Clients           int           `json:"clients"`
	Messages          int           `json:"messages"`
	Elapsed           time.Duration `json:"elapsed"`
	MessagesPerSecond float64       `json:"messages_per_second"`
}

func LoadRelayBundle(path string) (RelayBundle, error) {
	file, err := os.Open(path)
	if err != nil {
		return RelayBundle{}, fmt.Errorf("open relay load bundle: %w", err)
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, (1<<20)+1))
	decoder.DisallowUnknownFields()
	var bundle RelayBundle
	if err := decoder.Decode(&bundle); err != nil {
		return RelayBundle{}, fmt.Errorf("decode relay load bundle: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return RelayBundle{}, fmt.Errorf("relay load bundle contains trailing data")
	}
	if _, err := decodeRelayBundle(bundle); err != nil {
		return RelayBundle{}, err
	}
	return bundle, nil
}

func RunRelay(ctx context.Context, bundle RelayBundle, messagesPerParticipant int) (RelayResult, error) {
	decoded, err := decodeRelayBundle(bundle)
	if err != nil {
		return RelayResult{}, err
	}
	if messagesPerParticipant <= 0 || messagesPerParticipant > 10000 {
		return RelayResult{}, fmt.Errorf("relay load message count is invalid")
	}
	owner, err := relayclient.ConnectOwner(ctx, bundle.RelayURL, decoded.roomID, decoded.ownerPrivateKey)
	if err != nil {
		return RelayResult{}, err
	}
	defer func() { _ = owner.Close() }()
	admissions := make([]protocolv2.AdmissionControl, len(decoded.participants))
	for index, participant := range decoded.participants {
		var actor protocolv2.ActorKey
		copy(actor[:], participant.privateKey.Public().(ed25519.PublicKey))
		admissions[index] = protocolv2.AdmissionControl{Digest: sha256.Sum256(participant.admission[:]), Role: participant.role, ExpiresAt: time.Now().Add(10 * time.Minute), BoundActor: actor}
	}
	if err := owner.ReplaceAdmissions(ctx, 1, admissions); err != nil {
		return RelayResult{}, err
	}
	participants := make([]*relayclient.Client, 0, len(decoded.participants))
	defer func() {
		for _, participant := range participants {
			_ = participant.Close()
		}
	}()
	for index, value := range decoded.participants {
		participant, err := relayclient.ConnectParticipant(ctx, bundle.RelayURL, decoded.roomID, value.privateKey, value.admission)
		if err != nil {
			return RelayResult{}, fmt.Errorf("connect relay load participant %d: %w", index+1, err)
		}
		participants = append(participants, participant)
	}
	total := len(participants) * messagesPerParticipant
	readErrors := make(chan error, 1)
	go func() {
		for range total {
			_, message, err := owner.ReadApplication(ctx, decoded.groupKey)
			if err != nil {
				readErrors <- err
				return
			}
			if message.Type != protocolv2.ApplicationPresenceUpdate {
				readErrors <- fmt.Errorf("relay load received an unexpected application type")
				return
			}
		}
		readErrors <- nil
	}()
	started := time.Now()
	for sequence := 1; sequence <= messagesPerParticipant; sequence++ {
		for _, participant := range participants {
			payload := protocolv2.PresenceUpdate{Sequence: uint64(sequence), Status: "active"}
			if err := participant.SendApplication(ctx, decoded.groupKey, protocolv2.RouteOwner, protocolv2.ActorKey{}, protocolv2.ApplicationPresenceUpdate, payload); err != nil {
				return RelayResult{}, err
			}
		}
	}
	if err := <-readErrors; err != nil {
		return RelayResult{}, err
	}
	elapsed := time.Since(started)
	return RelayResult{Clients: len(participants), Messages: total, Elapsed: elapsed, MessagesPerSecond: float64(total) / elapsed.Seconds()}, nil
}

type decodedRelayBundle struct {
	roomID          protocolv2.RoomID
	ownerPrivateKey ed25519.PrivateKey
	groupKey        protocolv2.GroupKey
	participants    []decodedRelayParticipant
}

type decodedRelayParticipant struct {
	privateKey ed25519.PrivateKey
	admission  [32]byte
	role       protocolv2.Role
}

func decodeRelayBundle(bundle RelayBundle) (decodedRelayBundle, error) {
	if err := relayclient.ValidateEndpoint(bundle.RelayURL); err != nil {
		return decodedRelayBundle{}, err
	}
	room, err := decodeFixed(bundle.RoomID, len(protocolv2.RoomID{}))
	if err != nil {
		return decodedRelayBundle{}, fmt.Errorf("relay load room ID is invalid")
	}
	owner, err := decodeFixed(bundle.OwnerPrivateKey, ed25519.PrivateKeySize)
	if err != nil {
		return decodedRelayBundle{}, fmt.Errorf("relay load owner key is invalid")
	}
	group, err := decodeFixed(bundle.GroupKey, len(protocolv2.GroupKey{}))
	if err != nil {
		return decodedRelayBundle{}, fmt.Errorf("relay load group key is invalid")
	}
	if len(bundle.Participants) == 0 || len(bundle.Participants) > 64 {
		return decodedRelayBundle{}, fmt.Errorf("relay load participant count is invalid")
	}
	decoded := decodedRelayBundle{ownerPrivateKey: ed25519.PrivateKey(owner), participants: make([]decodedRelayParticipant, len(bundle.Participants))}
	copy(decoded.roomID[:], room)
	copy(decoded.groupKey[:], group)
	seen := make(map[string]struct{}, len(bundle.Participants))
	for index, participant := range bundle.Participants {
		privateKey, keyErr := decodeFixed(participant.PrivateKey, ed25519.PrivateKeySize)
		admission, admissionErr := decodeFixed(participant.Admission, 32)
		if keyErr != nil || admissionErr != nil || (participant.Role != protocolv2.RoleEditor && participant.Role != protocolv2.RoleViewer) {
			return decodedRelayBundle{}, fmt.Errorf("relay load participant %d is invalid", index+1)
		}
		actor := ed25519.PrivateKey(privateKey).Public().(ed25519.PublicKey)
		if _, duplicate := seen[string(actor)]; duplicate {
			return decodedRelayBundle{}, fmt.Errorf("relay load participant identity is duplicated")
		}
		seen[string(actor)] = struct{}{}
		decoded.participants[index] = decodedRelayParticipant{privateKey: ed25519.PrivateKey(privateKey), role: participant.Role}
		copy(decoded.participants[index].admission[:], admission)
	}
	return decoded, nil
}

func decodeFixed(value string, size int) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != size || bytes.Equal(decoded, make([]byte, size)) {
		return nil, fmt.Errorf("encoded value is invalid")
	}
	return decoded, nil
}
