package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	basicnet "github.com/livepeer/go-livepeer-basicnet"
	lpmscore "github.com/livepeer/lpms/core"
	"github.com/livepeer/lpms/transcoder"

	"github.com/ethereum/go-ethereum/common"
	"github.com/golang/glog"
	"github.com/livepeer/go-livepeer/core"
	eth "github.com/livepeer/go-livepeer/eth"
	lpmon "github.com/livepeer/go-livepeer/monitor"
	"github.com/livepeer/go-livepeer/net"
)

func (s *LivepeerServer) StartWebserver() {
	//Temporary endpoint just so we can invoke a transcode job.  IRL this should be invoked by transcoders monitoring the smart contract.
	http.HandleFunc("/transcode", func(w http.ResponseWriter, r *http.Request) {
		strmID := r.URL.Query().Get("strmID")
		if strmID == "" {
			http.Error(w, "Need to specify strmID", 500)
			return
		}

		ps := []lpmscore.VideoProfile{lpmscore.P240p30fps16x9, lpmscore.P360p30fps16x9}
		tr := transcoder.NewFFMpegSegmentTranscoder(ps, "", s.LivepeerNode.WorkDir)
		config := net.TranscodeConfig{StrmID: strmID, Profiles: ps}
		ids, err := s.LivepeerNode.TranscodeAndBroadcast(config, nil, tr)
		if err != nil {
			glog.Errorf("Error transcoding: %v", err)
			http.Error(w, "Error transcoding.", 500)
		}

		vids := make(map[core.StreamID]lpmscore.VideoProfile)
		for i, vp := range ps {
			vids[ids[i]] = vp
		}

		sid := core.StreamID(strmID)
		s.LivepeerNode.NotifyBroadcaster(sid.GetNodeID(), sid, vids)
	})

	//Set the broadcast config for creating onchain jobs.
	http.HandleFunc("/setBroadcastConfig", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			glog.Errorf("Parse Form Error: %v", err)
			return
		}

		priceStr := r.FormValue("maxPricePerSegment")
		if priceStr == "" {
			glog.Errorf("Need to provide max price per segment")
			return
		}
		price, err := strconv.Atoi(priceStr)
		if err != nil {
			glog.Errorf("Cannot convert max price per segment: %v", err)
			return
		}

		transcodingOptions := r.FormValue("transcodingOptions")
		if transcodingOptions == "" {
			glog.Errorf("Need to provide transcoding options")
			return
		}

		profiles := []lpmscore.VideoProfile{}
		for _, pName := range strings.Split(transcodingOptions, ",") {
			p, ok := lpmscore.VideoProfileLookup[pName]
			if ok {
				profiles = append(profiles, p)
			}
		}
		if len(profiles) == 0 {
			glog.Errorf("Invalid transcoding options: %v", transcodingOptions)
			return
		}

		BroadcastPrice = uint64(price)
		BroadcastJobVideoProfiles = profiles

		glog.Infof("Transcode Job Price: %v, Transcode Job Type: %v", BroadcastPrice, BroadcastJobVideoProfiles)
	})

	http.HandleFunc("/getBroadcastConfig", func(w http.ResponseWriter, r *http.Request) {
		pNames := []string{}
		for _, p := range BroadcastJobVideoProfiles {
			pNames = append(pNames, p.Name)
		}
		config := struct {
			MaxPricePerSegment uint64
			TranscodingOptions string
		}{
			BroadcastPrice,
			strings.Join(pNames, ","),
		}

		data, err := json.Marshal(config)
		if err != nil {
			glog.Errorf("Error marshalling broadcaster config: %v", err)
			return
		}

		w.Write(data)
	})

	http.HandleFunc("/getAvailableTranscodingOptions", func(w http.ResponseWriter, r *http.Request) {
		transcodingOptions := make([]string, 0, len(lpmscore.VideoProfileLookup))
		for opt := range lpmscore.VideoProfileLookup {
			transcodingOptions = append(transcodingOptions, opt)
		}

		data, err := json.Marshal(transcodingOptions)
		if err != nil {
			glog.Errorf("Error marshalling all transcoding options: %v", err)
			return
		}

		w.Write(data)
	})

	//Activate the transcoder on-chain.
	http.HandleFunc("/activateTranscoder", func(w http.ResponseWriter, r *http.Request) {
		accountAddr := s.LivepeerNode.Eth.Account().Address
		bondingManager := s.LivepeerNode.Eth.BondingManager()

		t, err := bondingManager.getTranscoder(accountAddr)
		registered, err := s.LivepeerNode.Eth.IsRegisteredTranscoder()
		if err != nil {
			glog.Errorf("Error checking for registered transcoder: %v", err)
			return
		}

		if registered {
			glog.Error("Transcoder is already registered")
			return
		}

		if err := r.ParseForm(); err != nil {
			glog.Errorf("Parse Form Error: %v", err)
			return
		}

		blockRewardCutStr := r.FormValue("blockRewardCut")
		if blockRewardCutStr == "" {
			glog.Errorf("Need to provide block reward cut")
			return
		}
		blockRewardCut, err := strconv.Atoi(blockRewardCutStr)
		if err != nil {
			glog.Errorf("Cannot convert block reward cut: %v", err)
			return
		}

		feeShareStr := r.FormValue("feeShare")
		if feeShareStr == "" {
			glog.Errorf("Need to provide fee share")
			return
		}
		feeShare, err := strconv.Atoi(feeShareStr)
		if err != nil {
			glog.Errorf("Cannot convert fee share: %v", err)
			return
		}

		priceStr := r.FormValue("pricePerSegment")
		if priceStr == "" {
			glog.Errorf("Need to provide price per segment")
			return
		}
		price, err := strconv.Atoi(priceStr)
		if err != nil {
			glog.Errorf("Cannot convert price per segment: %v", err)
			return
		}

		amountStr := r.FormValue("amount")
		if amountStr == "" {
			glog.Errorf("Need to provide amount")
			return
		}
		amount, err := strconv.Atoi(amountStr)
		if err != nil {
			glog.Errorf("Cannot convert amount: %v", err)
			return
		}

		if err := eth.CheckRoundAndInit(s.LivepeerNode.Eth); err != nil {
			glog.Errorf("Error checking and initializing round: %v", err)
			return
		}

		if amount > 0 {
			glog.Infof("Bonding %v...", amount)
			bondRc, bondEc := s.LivepeerNode.Eth.Bond(big.NewInt(int64(amount)), s.LivepeerNode.Eth.Account().Address)
			select {
			case <-bondRc:
				glog.Infof("Activating Transcoder %v", s.LivepeerNode.Eth.Account().Address)
				rc, ec := s.LivepeerNode.Eth.Transcoder(big.NewInt(int64(blockRewardCut*10000)), big.NewInt(int64(feeShare*10000)), big.NewInt(int64(price)))
				select {
				case rec := <-rc:
					glog.Infof("%v", rec)
				case err := <-ec:
					glog.Errorf("Error creating transcoder: %v", err)
				}
			case err := <-bondEc:
				glog.Errorf("Error bonding: %v", err)
			}
		} else {
			glog.Infof("Activating Transcoder %v", s.LivepeerNode.Eth.Account().Address)
			rc, ec := s.LivepeerNode.Eth.Transcoder(big.NewInt(int64(blockRewardCut*10000)), big.NewInt(int64(feeShare*10000)), big.NewInt(int64(price)))
			select {
			case rec := <-rc:
				glog.Infof("%v", rec)
			case err := <-ec:
				glog.Errorf("Error creating transcoder: %v", err)
			}
		}
	})

	//Set transcoder config on-chain.
	http.HandleFunc("/setTranscoderConfig", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			glog.Errorf("Parse Form Error: %v", err)
			return
		}

		blockRewardCutStr := r.FormValue("blockRewardCut")
		if blockRewardCutStr == "" {
			glog.Errorf("Need to provide block reward cut")
			return
		}
		blockRewardCut, err := strconv.Atoi(blockRewardCutStr)
		if err != nil {
			glog.Errorf("Cannot convert block reward cut: %v", err)
			return
		}

		feeShareStr := r.FormValue("feeShare")
		if feeShareStr == "" {
			glog.Errorf("Need to provide fee share")
			return
		}
		feeShare, err := strconv.Atoi(feeShareStr)
		if err != nil {
			glog.Errorf("Cannot convert fee share: %v", err)
			return
		}

		priceStr := r.FormValue("pricePerSegment")
		if priceStr == "" {
			glog.Errorf("Need to provide price per segment")
			return
		}
		price, err := strconv.Atoi(priceStr)
		if err != nil {
			glog.Errorf("Cannot convert price per segment: %v", err)
			return
		}

		if err := eth.CheckRoundAndInit(s.LivepeerNode.Eth); err != nil {
			glog.Errorf("Error checking and initializing round: %v", err)
			return
		}

		rc, ec := s.LivepeerNode.Eth.Transcoder(big.NewInt(int64(blockRewardCut*10000)), big.NewInt(int64(feeShare*10000)), big.NewInt(int64(price)))
		select {
		case rec := <-rc:
			glog.Infof("%v", rec)
		case err := <-ec:
			glog.Errorf("Error setting transcoder config: %v", err)
		}
	})

	//Bond some amount of tokens to a transcoder.
	http.HandleFunc("/bond", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			if err := r.ParseForm(); err != nil {
				glog.Errorf("Parse Form Error: %v", err)
				return
			}

			amountStr := r.FormValue("amount")
			if amountStr == "" {
				glog.Errorf("Need to provide amount")
				return
			}
			amount, err := strconv.Atoi(amountStr)
			if err != nil {
				glog.Errorf("Cannot convert amount: %v", err)
				return
			}

			toAddr := r.FormValue("toAddr")
			if toAddr == "" {
				glog.Errorf("Need to provide to addr")
				return
			}

			if err := eth.CheckRoundAndInit(s.LivepeerNode.Eth); err != nil {
				glog.Errorf("Error checking and initializing round: %v", err)
				return
			}

			rc, ec := s.LivepeerNode.Eth.Bond(big.NewInt(int64(amount)), common.HexToAddress(toAddr))
			select {
			case rec := <-rc:
				glog.Infof("%v", rec)
			case err := <-ec:
				glog.Errorf("Error bonding: %v", err)
			}
		}
	})

	http.HandleFunc("/unbond", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			if err := eth.CheckRoundAndInit(s.LivepeerNode.Eth); err != nil {
				glog.Errorf("Error checking and initializing round: %v", err)
				return
			}

			rc, ec := s.LivepeerNode.Eth.Unbond()
			select {
			case rec := <-rc:
				glog.Infof("%v", rec)
			case err := <-ec:
				glog.Errorf("Error unbonding: %v", err)
			}
		}
	})

	http.HandleFunc("/withdrawBond", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			if err := eth.CheckRoundAndInit(s.LivepeerNode.Eth); err != nil {
				glog.Errorf("Error checking and initializing round: %v", err)
				return
			}

			rc, ec := s.LivepeerNode.Eth.WithdrawBond()
			select {
			case rec := <-rc:
				glog.Infof("%v", rec)
			case err := <-ec:
				glog.Errorf("Error withdrawing bond: %v", err)
			}
		}
	})

	http.HandleFunc("/transcoderStatus", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			status, err := s.LivepeerNode.Eth.TranscoderStatus()
			if err != nil {
				w.Write([]byte(""))
			}
			w.Write([]byte(status))
		}
	})

	//Print the transcoder's stake
	http.HandleFunc("/transcoderStake", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			b, err := s.LivepeerNode.Eth.TranscoderStake()
			if err != nil {
				w.Write([]byte(""))
			}
			w.Write([]byte(b.String()))
		}
	})

	http.HandleFunc("/delegatorStatus", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			status, err := s.LivepeerNode.Eth.DelegatorStatus()
			if err != nil {
				w.Write([]byte(""))
			}
			w.Write([]byte(status))
		}
	})

	http.HandleFunc("/delegatorStake", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			s, err := s.LivepeerNode.Eth.DelegatorStake()
			if err != nil {
				w.Write([]byte(""))
			}
			w.Write([]byte(s.String()))
		}
	})

	http.HandleFunc("/deposit", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			if err := r.ParseForm(); err != nil {
				glog.Errorf("Parse Form Error: %v", err)
				return
			}
			//Parse amount
			amountStr := r.FormValue("amount")
			if amountStr == "" {
				glog.Errorf("Need to provide amount")
				return
			}
			amount, err := strconv.Atoi(amountStr)
			if err != nil {
				glog.Errorf("Cannot convert amount: %v", err)
				return
			}
			glog.Infof("Depositing: %v", amount)

			rc, ec := s.LivepeerNode.Eth.Deposit(big.NewInt(int64(amount)))
			select {
			case <-rc:
				glog.Infof("Deposit successful")
			case err := <-ec:
				glog.Errorf("Error depositing: %v", err)
			}
		}
	})

	//Print the current broadcast HLS streamID
	http.HandleFunc("/streamID", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(LastHLSStreamID))
	})

	http.HandleFunc("/manifestID", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(LastManifestID))
	})

	http.HandleFunc("/localStreams", func(w http.ResponseWriter, r *http.Request) {
		net := s.LivepeerNode.VideoNetwork.(*basicnet.BasicVideoNetwork)
		ret := make([]map[string]string, 0)
		for _, strmID := range net.GetLocalStreams() {
			ret = append(ret, map[string]string{"format": "hls", "streamID": strmID})
		}
		js, err := json.Marshal(ret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	})

	http.HandleFunc("/peersCount", func(w http.ResponseWriter, r *http.Request) {
		ret := make(map[string]int)
		ret["count"] = lpmon.Instance().GetPeerCount()

		js, err := json.Marshal(ret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	})

	http.HandleFunc("/debug", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fmt.Sprintf("\n\nVideoNetwork: %v", s.LivepeerNode.VideoNetwork)))
		w.Write([]byte(fmt.Sprintf("\n\nmediaserver sub timer: %v", s.hlsSubTimer)))
	})

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		nid := r.FormValue("nodeID")

		if nid == "" {
			nid = string(s.LivepeerNode.Identity)
		}

		statusc, err := s.LivepeerNode.VideoNetwork.GetNodeStatus(nid)
		if err == nil {
			status := <-statusc
			mstrs := make(map[string]string, 0)
			for mid, m := range status.Manifests {
				mstrs[mid] = m.String()
			}
			d := struct {
				NodeID    string
				Manifests map[string]string
			}{
				NodeID:    status.NodeID,
				Manifests: mstrs,
			}
			if data, err := json.Marshal(d); err == nil {
				w.Header().Set("Content-Type", "application/json")
				w.Write(data)
				return
			}
		}
	})

	http.HandleFunc("/nodeID", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(s.LivepeerNode.VideoNetwork.GetNodeID()))
	})

	http.HandleFunc("/nodeAddrs", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Join(s.LivepeerNode.Addrs, ", ")))
	})

	http.HandleFunc("/controllerContractAddr", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			w.Write([]byte(s.LivepeerNode.Eth.GetControllerAddr()))
		}
	})

	http.HandleFunc("/tokenContractAddr", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			w.Write([]byte(s.LivepeerNode.Eth.GetTokenAddr()))
		}
	})

	http.HandleFunc("/faucetContractAddr", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			w.Write([]byte(s.LivepeerNode.Eth.GetFaucetAddr()))
		}
	})

	http.HandleFunc("/ethAddr", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			w.Write([]byte(s.LivepeerNode.EthAccount))
		}
	})

	http.HandleFunc("/tokenBalance", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			b, err := s.LivepeerNode.Eth.TokenBalance()
			if err != nil {
				w.Write([]byte(""))
			}
			w.Write([]byte(b.String()))
		}
	})

	http.HandleFunc("/ethBalance", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			b, err := s.LivepeerNode.Eth.Backend().BalanceAt(context.Background(), s.LivepeerNode.Eth.Account().Address, nil)
			if err != nil {
				w.Write([]byte(""))
			}
			w.Write([]byte(b.String()))
		}
	})

	http.HandleFunc("/broadcasterDeposit", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			b, err := s.LivepeerNode.Eth.GetBroadcasterDeposit(s.LivepeerNode.Eth.Account().Address)
			if err != nil {
				w.Write([]byte(""))
			}
			w.Write([]byte(b.String()))
		}
	})

	http.HandleFunc("/transcoderBond", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			b, err := s.LivepeerNode.Eth.TranscoderBond()
			if err != nil {
				w.Write([]byte(""))
			}
			w.Write([]byte(b.String()))
		}
	})

	http.HandleFunc("/isActiveTranscoder", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			reg, err := s.LivepeerNode.Eth.IsRegisteredTranscoder()
			if err != nil {
				w.Write([]byte("False"))
				return
			}
			active, err := s.LivepeerNode.Eth.IsActiveTranscoder()
			if err != nil {
				w.Write([]byte("False"))
				return
			}

			if reg && active {
				w.Write([]byte("True"))
			} else {
				w.Write([]byte("False"))
			}
			return
		}

		w.Write([]byte("False"))
	})

	http.HandleFunc("/candidateTranscodersStats", func(w http.ResponseWriter, r *http.Request) {
		candidateTranscodersStats, err := s.LivepeerNode.Eth.GetCandidateTranscodersStats()
		if err != nil {
			w.Write([]byte(""))
		}

		data, err := json.Marshal(candidateTranscodersStats)
		if err != nil {
			glog.Errorf("Error marshalling all transcoder stats: %v", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	http.HandleFunc("/transcoderPendingBlockRewardCut", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			blockRewardCut, _, _, err := s.LivepeerNode.Eth.TranscoderPendingPricingInfo()
			if err != nil {
				w.Write([]byte(""))
			}
			w.Write([]byte(strconv.Itoa(int(blockRewardCut.Int64()))))
		}
	})

	http.HandleFunc("/transcoderPendingFeeShare", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			_, feeShare, _, err := s.LivepeerNode.Eth.TranscoderPendingPricingInfo()
			if err != nil {
				w.Write([]byte(""))
			}
			w.Write([]byte(strconv.Itoa(int(feeShare.Int64()))))
		}
	})

	http.HandleFunc("/transcoderPendingPrice", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			_, _, price, err := s.LivepeerNode.Eth.TranscoderPendingPricingInfo()
			if err != nil {
				w.Write([]byte(""))
			}
			w.Write([]byte(price.String()))
		}
	})

	http.HandleFunc("/transcoderBlockRewardCut", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			blockRewardCut, _, _, err := s.LivepeerNode.Eth.TranscoderPricingInfo()
			if err != nil {
				w.Write([]byte(""))
			}
			w.Write([]byte(strconv.Itoa(int(blockRewardCut.Int64()))))
		}
	})

	http.HandleFunc("/transcoderFeeShare", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			_, feeShare, _, err := s.LivepeerNode.Eth.TranscoderPricingInfo()
			if err != nil {
				w.Write([]byte(""))
			}
			w.Write([]byte(strconv.Itoa(int(feeShare.Int64()))))
		}
	})

	http.HandleFunc("/transcoderPrice", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			_, _, price, err := s.LivepeerNode.Eth.TranscoderPricingInfo()
			if err != nil {
				w.Write([]byte(""))
			}
			w.Write([]byte(price.String()))
		}
	})

	http.HandleFunc("/requestTokens", func(w http.ResponseWriter, r *http.Request) {
		if s.LivepeerNode.Eth != nil {
			glog.Infof("Requesting tokens from faucet")

			rc, ec := s.LivepeerNode.Eth.RequestTokens()
			select {
			case rec := <-rc:
				glog.Infof("%v", rec)
			case err := <-ec:
				glog.Errorf("Error request tokens from faucet: %v", err)
			}
		}
	})
}
