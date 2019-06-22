package webrtc

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStatsTimestampTime(t *testing.T) {
	for _, test := range []struct {
		Timestamp StatsTimestamp
		WantTime  time.Time
	}{
		{
			Timestamp: 0,
			WantTime:  time.Unix(0, 0),
		},
		{
			Timestamp: 1,
			WantTime:  time.Unix(0, 1e6),
		},
		{
			Timestamp: 0.001,
			WantTime:  time.Unix(0, 1e3),
		},
	} {
		if got, want := test.Timestamp.Time(), test.WantTime.UTC(); got != want {
			t.Fatalf("StatsTimestamp(%v).Time() = %v, want %v", test.Timestamp, got, want)
		}
	}
}

// TODO(maxhawkins): replace with a more meaningful test
func TestStatsMarshal(t *testing.T) {
	for _, test := range []Stats{
		AudioReceiverStats{},
		AudioSenderStats{},
		CertificateStats{},
		CodecStats{},
		DataChannelStats{},
		ICECandidatePairStats{},
		ICECandidateStats{},
		InboundRTPStreamStats{},
		MediaStreamStats{},
		OutboundRTPStreamStats{},
		PeerConnectionStats{},
		RemoteInboundRTPStreamStats{},
		RemoteOutboundRTPStreamStats{},
		RTPContributingSourceStats{},
		SenderAudioTrackAttachmentStats{},
		SenderAudioTrackAttachmentStats{},
		SenderVideoTrackAttachmentStats{},
		TransportStats{},
		VideoReceiverStats{},
		VideoReceiverStats{},
		VideoSenderStats{},
	} {
		_, err := json.Marshal(test)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func waitWithTimeout(t *testing.T, wg *sync.WaitGroup) {
	// Wait for all of the event handlers to be triggered.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		done <- struct{}{}
	}()
	timeout := time.After(5 * time.Second)
	select {
	case <-done:
		break
	case <-timeout:
		t.Fatal("timed out waiting for waitgroup")
	}
}

func getConnectionStats(t *testing.T, report StatsReport, ID string) PeerConnectionStats {
	assert.Contains(t, report, ID)
	stats, ok := report[ID].(PeerConnectionStats)
	assert.True(t, ok)
	assert.Equal(t, stats.Type, StatsTypePeerConnection)
	return stats
}

func getDataChannelStats(t *testing.T, report StatsReport, ID string) DataChannelStats {
	assert.Contains(t, report, ID)
	stats, ok := report[ID].(DataChannelStats)
	assert.True(t, ok)
	assert.Equal(t, stats.Type, StatsTypeDataChannel)
	return stats
}

func findLocalCandidateStats(t *testing.T, report StatsReport) []ICECandidateStats {
	result := []ICECandidateStats{}
	for _, s := range report {
		stats, ok := s.(ICECandidateStats)
		if ok && stats.Type == StatsTypeLocalCandidate {
			result = append(result, stats)
		}
	}
	return result
}

func findRemoteCandidateStats(t *testing.T, report StatsReport) []ICECandidateStats {
	result := []ICECandidateStats{}
	for _, s := range report {
		stats, ok := s.(ICECandidateStats)
		if ok && stats.Type == StatsTypeRemoteCandidate {
			result = append(result, stats)
		}
	}
	return result
}

func findCandidatePairStats(t *testing.T, report StatsReport) []ICECandidatePairStats {
	result := []ICECandidatePairStats{}
	for _, s := range report {
		stats, ok := s.(ICECandidatePairStats)
		if ok {
			assert.Equal(t, StatsTypeCandidatePair, stats.Type)
			result = append(result, stats)
		}
	}
	return result
}

func signalPairForStats(pcOffer *PeerConnection, pcAnswer *PeerConnection) error {
	offerChan := make(chan SessionDescription)
	pcOffer.OnICECandidate(func(candidate *ICECandidate) {
		if candidate == nil {
			offerChan <- *pcOffer.PendingLocalDescription()
		}
	})

	offer, err := pcOffer.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := pcOffer.SetLocalDescription(offer); err != nil {
		return err
	}

	timeout := time.After(3 * time.Second)
	select {
	case <-timeout:
		return fmt.Errorf("timed out waiting to receive offer")
	case offer := <-offerChan:
		if err := pcAnswer.SetRemoteDescription(offer); err != nil {
			return err
		}

		answer, err := pcAnswer.CreateAnswer(nil)
		if err != nil {
			return err
		}

		if err = pcAnswer.SetLocalDescription(answer); err != nil {
			return err
		}

		err = pcOffer.SetRemoteDescription(answer)
		if err != nil {
			return err
		}
		return nil
	}
}

func TestPeerConnection_GetStats(t *testing.T) {
	offerPC, answerPC, err := newPair()
	assert.NoError(t, err)

	baseLineReportPCOffer := offerPC.GetStats()
	baseLineReportPCAnswer := answerPC.GetStats()

	connStatsOffer := getConnectionStats(t, baseLineReportPCOffer, offerPC.ObjectID())
	connStatsAnswer := getConnectionStats(t, baseLineReportPCAnswer, answerPC.ObjectID())

	for _, connStats := range []PeerConnectionStats{connStatsOffer, connStatsAnswer} {
		assert.Equal(t, uint32(0), connStats.DataChannelsOpened)
		assert.Equal(t, uint32(0), connStats.DataChannelsClosed)
		assert.Equal(t, uint32(0), connStats.DataChannelsRequested)
		assert.Equal(t, uint32(0), connStats.DataChannelsAccepted)
	}

	// Create an open a DC
	offerDC, err := offerPC.CreateDataChannel("offerDC", nil)
	assert.NoError(t, err)
	dcWait := sync.WaitGroup{}
	dcWait.Add(1)
	var answerDC *DataChannel
	answerPC.OnDataChannel(func(d *DataChannel) {
		d.OnOpen(func() {
			answerDC = d
			dcWait.Done()
		})
	})
	assert.NoError(t, signalPairForStats(offerPC, answerPC))
	waitWithTimeout(t, &dcWait)

	reportPCOffer := offerPC.GetStats()
	reportPCAnswer := answerPC.GetStats()

	connStatsOffer = getConnectionStats(t, reportPCOffer, offerPC.ObjectID())
	assert.Equal(t, uint32(1), connStatsOffer.DataChannelsOpened)
	assert.Equal(t, uint32(0), connStatsOffer.DataChannelsClosed)
	assert.Equal(t, uint32(1), connStatsOffer.DataChannelsRequested)
	assert.Equal(t, uint32(0), connStatsOffer.DataChannelsAccepted)
	dcStatsOffer := getDataChannelStats(t, reportPCOffer, offerDC.ObjectID())
	assert.Equal(t, DataChannelStateOpen, dcStatsOffer.State)
	assert.NotEmpty(t, findLocalCandidateStats(t, reportPCOffer))
	assert.NotEmpty(t, findRemoteCandidateStats(t, reportPCOffer))
	assert.NotEmpty(t, findCandidatePairStats(t, reportPCOffer))

	connStatsAnswer = getConnectionStats(t, reportPCAnswer, answerPC.ObjectID())
	assert.Equal(t, uint32(1), connStatsAnswer.DataChannelsOpened)
	assert.Equal(t, uint32(0), connStatsAnswer.DataChannelsClosed)
	assert.Equal(t, uint32(0), connStatsAnswer.DataChannelsRequested)
	assert.Equal(t, uint32(1), connStatsAnswer.DataChannelsAccepted)
	dcStatsAnswer := getDataChannelStats(t, reportPCOffer, answerDC.ObjectID())
	assert.Equal(t, DataChannelStateOpen, dcStatsAnswer.State)
	assert.NotEmpty(t, findLocalCandidateStats(t, reportPCAnswer))
	assert.NotEmpty(t, findRemoteCandidateStats(t, reportPCAnswer))
	assert.NotEmpty(t, findCandidatePairStats(t, reportPCAnswer))

	// Close answer DC now
	dcWait = sync.WaitGroup{}
	dcWait.Add(1)
	offerDC.OnClose(func() {
		dcWait.Done()
	})
	assert.NoError(t, answerDC.Close())
	waitWithTimeout(t, &dcWait)

	reportPCOffer = offerPC.GetStats()
	reportPCAnswer = answerPC.GetStats()

	connStatsOffer = getConnectionStats(t, reportPCOffer, offerPC.ObjectID())
	assert.Equal(t, uint32(1), connStatsOffer.DataChannelsOpened)
	assert.Equal(t, uint32(1), connStatsOffer.DataChannelsClosed)
	assert.Equal(t, uint32(1), connStatsOffer.DataChannelsRequested)
	assert.Equal(t, uint32(0), connStatsOffer.DataChannelsAccepted)
	dcStatsOffer = getDataChannelStats(t, reportPCOffer, offerDC.ObjectID())
	assert.Equal(t, DataChannelStateClosed, dcStatsOffer.State)

	connStatsAnswer = getConnectionStats(t, reportPCAnswer, answerPC.ObjectID())
	assert.Equal(t, uint32(1), connStatsAnswer.DataChannelsOpened)
	assert.Equal(t, uint32(1), connStatsAnswer.DataChannelsClosed)
	assert.Equal(t, uint32(0), connStatsAnswer.DataChannelsRequested)
	assert.Equal(t, uint32(1), connStatsAnswer.DataChannelsAccepted)
	dcStatsAnswer = getDataChannelStats(t, reportPCOffer, answerDC.ObjectID())
	assert.Equal(t, DataChannelStateClosed, dcStatsAnswer.State)

	assert.NoError(t, offerPC.Close())
	assert.NoError(t, answerPC.Close())
}
