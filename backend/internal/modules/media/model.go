package media

// uploadBody is POST /media/uploads and POST /admin/media/uploads.
type uploadBody struct {
	Purpose     string `json:"purpose"`
	ContentType string `json:"contentType"`
	SizeBytes   int64  `json:"sizeBytes"`
}

// Confirmed is the response of POST /media/uploads/:id/confirm.
type Confirmed struct {
	UploadID int64  `json:"uploadId"`
	Status   string `json:"status"`
}

// pendingRow is an unconfirmed upload picked up by the cleanup job.
type pendingRow struct {
	ID  int64
	Key string
}
