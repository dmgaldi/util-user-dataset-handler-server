package job

import (
	"net/http"
	"os"

	"github.com/Foxcapades/go-midl/v2/pkg/midl"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"

	"github.com/VEuPathDB/util-exporter-server/internal/command"
	"github.com/VEuPathDB/util-exporter-server/internal/config"
	"github.com/VEuPathDB/util-exporter-server/internal/except"
	"github.com/VEuPathDB/util-exporter-server/internal/job"
	"github.com/VEuPathDB/util-exporter-server/internal/server/middle"
	"github.com/VEuPathDB/util-exporter-server/internal/server/svc"
	"github.com/VEuPathDB/util-exporter-server/internal/server/types"
	"github.com/VEuPathDB/util-exporter-server/internal/service/cache"
	"github.com/VEuPathDB/util-exporter-server/internal/service/logger"
	"github.com/VEuPathDB/util-exporter-server/internal/service/workspace"
	"github.com/VEuPathDB/util-exporter-server/internal/util"
)

var (
	promRequestPayloadSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "upload",
		Name:      "file_size",
		Help:      "File size for user uploads in MiB.",
		Buckets: []float64{
			0.5,  // 0.5 MiB
			1,    // 1   MiB
			10,   // 10  MiB
			50,   // 50  MiB
			100,  // 100 MiB
			250,  // 250 MiB
			500,  // 500 MiB
			1024, // 1   GiB
		},
	}, []string{"ext"})
)

// NewUploadEndpoint instantiates a new endpoint wrapper for the user dataset
// upload handler.
func NewUploadEndpoint(file config.FileOptions) types.Endpoint {
	return &uploadEndpoint{file: file}
}

type uploadEndpoint struct {
	log  *logrus.Entry
	file config.FileOptions
}

func (u *uploadEndpoint) Register(r *mux.Router) {
	r.Path(urlPath).
		Methods(http.MethodPost).
		Handler(middle.MetricAgg(middle.RequestCtxProvider(
			middle.BinaryAdaptor().AddHandlers(
				middle.JobIDValidator(tokenKey, u)))))
}

// Handle the request.
//
// If we've made it this far we know that the token in the URL is valid and
// points to an existing metadata entry in the store.
func (u *uploadEndpoint) Handle(req midl.Request) midl.Response {
	log := logger.ByRequest(req)
	u.log = log

	log.Trace("uploadEndpoint#handle")

	token := mux.Vars(req.RawRequest())[tokenKey]
	meta := u.getMeta(token)
	dets := u.createDetails(&meta)

	wkspc, err := workspace.Create(token, log)
	if err != nil {
		log.WithField("status", http.StatusInternalServerError).Error(err)
		return svc.ServerError(err.Error())
	}

	if res := u.HandleUpload(req, dets, wkspc); res != nil {
		return res
	}

	result := command.NewCommandRunner(token, u.file, wkspc, log).Run()
	if result.Error != nil {
		switch result.Error.(type) {
		case *command.UserError:
			log.WithField("status", http.StatusUnprocessableEntity).Error(result.Error)
			return svc.InvalidRequest(result.Error.Error()).Callback(u.cleanup(token))
		default:
			log.WithField("status", http.StatusInternalServerError).Error(result.Error)
			return svc.ServerError(result.Error.Error()).Callback(u.cleanup(token))
		}
	}

	return midl.MakeResponse(http.StatusOK, result).Callback(u.cleanup(token))
}

func (u *uploadEndpoint) HandleUpload(
	request midl.Request,
	details *job.Details,
	wkspc workspace.Workspace,
) midl.Response {
	u.log.Trace("uploadEndpoint#HandleUpload")

	fileName, stream, res := GetFileHandle(request.RawRequest(), u.log)
	if res != nil {
		return u.FailJob(res, details)
	}
	defer stream.Close()

	suff, errRes := u.ValidateFileSuffix(fileName, u.log)
	if errRes != nil {
		return u.FailJob(errRes, details)
	}

	details.WorkingDir = wkspc.GetPath()
	u.storeDetails(details)

	file, err := wkspc.FileFromUpload(fileName, stream)
	if err != nil {
		u.log.WithField("status", http.StatusInternalServerError).Error(err)
		return svc.ServerError(err.Error())
	}
	defer file.Close()

	info, err := file.Stat()

	if err != nil {
		u.log.WithField("status", http.StatusInternalServerError).Error(err)
		return svc.ServerError(except.NewServerError(err.Error()).Error())
	}

	promRequestPayloadSize.WithLabelValues(suff).
		Observe(float64(info.Size()) / float64(util.SizeMebibyte))

	details.InTarName = fileName
	u.storeDetails(details)

	return nil
}

// retrieve metadata from the metadata store.
func (u *uploadEndpoint) getMeta(token string) job.Metadata {
	tmp, _ := cache.GetMetadata(token)
	return tmp
}

// remove the working directory and convert the stored metadata to the long
// store form.
func (u *uploadEndpoint) cleanup(token string) func() {
	return func() {
		u.log.Debug("cleaning up workspace")

		details, _ := cache.GetDetails(token)

		_ = os.RemoveAll(details.WorkingDir)
		cache.PutHistoricalDetails(token, details.StorableDetails)
		cache.DeleteMetadata(token)
		cache.DeleteDetails(token)
	}
}
