package server

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/golang/glog"
	"github.com/livepeer/go-livepeer/common"
	"github.com/livepeer/go-livepeer/eth"
)

func logAndRespondWithError(w http.ResponseWriter, errMsg string, code int) {
	glog.Error(errMsg)
	http.Error(w, errMsg, code)
}

func mustHaveFormParams(h http.Handler, params ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			glog.Error(err)
			logAndRespondWithError(w, "parse form error", http.StatusInternalServerError)
			return
		}

		for _, param := range params {
			if r.FormValue(param) == "" {
				logAndRespondWithError(w, fmt.Sprintf("missing form param: %s", param), http.StatusBadRequest)
				return
			}
		}

		h.ServeHTTP(w, r)
	})
}

func fundAndApproveSignersHandler(client eth.LivepeerEthClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			logAndRespondWithError(w, "missing ETH client", http.StatusInternalServerError)
			return
		}

		amount, err := common.ParseBigInt(r.FormValue("amount"))
		if err != nil {
			logAndRespondWithError(w, "invalid amount", http.StatusBadRequest)
			return
		}

		penaltyEscrowAmount, err := client.MinPenaltyEscrow()
		if err != nil {
			logAndRespondWithError(w, "could not execute fundAndApproveSigners", http.StatusInternalServerError)
			return
		}

		if amount.Cmp(penaltyEscrowAmount) < 0 {
			logAndRespondWithError(w, "amount is not sufficient for minimum penalty escrow", http.StatusBadRequest)
			return
		}

		depositAmount := new(big.Int).Sub(amount, penaltyEscrowAmount)

		tx, err := client.FundAndApproveSigners(depositAmount, penaltyEscrowAmount, []ethcommon.Address{})
		if err != nil {
			glog.Error(err)
			logAndRespondWithError(w, "could not execute fundAndApproveSigners", http.StatusInternalServerError)
			return
		}

		err = client.CheckTx(tx)
		if err != nil {
			glog.Error(err)
			logAndRespondWithError(w, "could not execute fundAndApproveSigners", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fundAndApproveSigners success"))
	})
}

func fundDepositHandler(client eth.LivepeerEthClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			logAndRespondWithError(w, "missing ETH client", http.StatusInternalServerError)
			return
		}

		amount, err := common.ParseBigInt(r.FormValue("amount"))
		if err != nil {
			logAndRespondWithError(w, "invalid amount", http.StatusBadRequest)
			return
		}

		tx, err := client.FundDeposit(amount)
		if err != nil {
			logAndRespondWithError(w, "could not execute fundDeposit", http.StatusInternalServerError)
			return
		}

		err = client.CheckTx(tx)
		if err != nil {
			logAndRespondWithError(w, "could not execute fundDeposit", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fundDeposit success"))
	})
}

func senderInfoHandler(client eth.LivepeerEthClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			logAndRespondWithError(w, "missing ETH client", http.StatusInternalServerError)
			return
		}

		sender, err := client.Senders(client.Account().Address)
		if err != nil {
			glog.Error(err)
			logAndRespondWithError(w, "could not query sender info", http.StatusInternalServerError)
			return
		}

		data, err := json.Marshal(sender)
		if err != nil {
			glog.Error(err)
			logAndRespondWithError(w, "could not query sender info", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})
}
