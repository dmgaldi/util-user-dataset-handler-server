package middle

import (
	"github.com/Foxcapades/go-midl/v2/pkg/midl"
	"github.com/VEuPathDB/util-exporter-server/internal/server/svc"
	"github.com/gorilla/mux"
)

func RegisterGenericHandlers(r *mux.Router) {
	r.MethodNotAllowedHandler = midl.JSONAdapter(
		RequestCtxProvider(), NewTimer(svc.New405Handler()))
	r.NotFoundHandler = midl.JSONAdapter(
		RequestCtxProvider(), NewTimer(svc.New404Handler()))
}

