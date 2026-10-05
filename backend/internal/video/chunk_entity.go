package video

const ChunkSize = 5 << 20 // 5 MB

// ChunkUploadSession 保存在 Redis 中的分片上传会话，用于断点续传和确认上传者身份。
type ChunkUploadSession struct {
	UploadID     string `json:"upload_id"`     // 本次上传会话的唯一标识
	AccountID    uint   `json:"account_id"`    // 创建上传会话的账户 ID；其他用户不能上传或合并该会话
	Filename     string `json:"filename"`      // 客户端提供的原始文件名；最终存储名由服务端生成
	FileSize     int64  `json:"file_size"`     // 客户端声明的完整文件大小，单位字节
	ChunkSize    int64  `json:"chunk_size"`    // 客户端声明的每片大小，单位字节
	TotalChunks  int    `json:"total_chunks"`  // 客户端声明的分片总数
	FileHash     string `json:"file_hash"`     // 客户端提供的完整文件哈希，用于同账户断点续传查找；当前合并流程不校验整文件哈希
	UploadedBits []bool `json:"uploaded_bits"` // 分片完成标记；数组下标即从 0 开始的分片序号
}

func (s *ChunkUploadSession) UploadedChunks() []int {
	var indices []int
	for i, uploaded := range s.UploadedBits {
		if uploaded {
			indices = append(indices, i)
		}
	}
	return indices
}

func (s *ChunkUploadSession) IsComplete() bool {
	for _, b := range s.UploadedBits {
		if !b {
			return false
		}
	}
	return true
}

// InitChunkUploadRequest 初始化或恢复分片上传；同一账户与文件哈希可复用已有会话。
type InitChunkUploadRequest struct {
	Filename    string `json:"filename" binding:"required"`           // 待上传文件的原始名称
	FileSize    int64  `json:"file_size" binding:"required,min=1"`    // 完整文件大小，单位字节；当前上限为 200 MiB
	ChunkSize   int64  `json:"chunk_size" binding:"required,min=1"`   // 客户端规划的分片大小，单位字节
	TotalChunks int    `json:"total_chunks" binding:"required,min=1"` // 客户端规划的分片总数，应与文件大小和分片大小一致
	FileHash    string `json:"file_hash" binding:"required"`          // 客户端计算的整文件哈希；作为续传标识，当前服务端不校验最终整文件哈希
}

// UploadChunkRequest 上传单片的 multipart 表单元数据；分片二进制另放在 file 表单项。
type UploadChunkRequest struct {
	UploadID   string `form:"upload_id" binding:"required"`  // 初始化接口返回的上传会话标识
	ChunkIndex int    `form:"chunk_index" binding:"min=0"`   // 本片序号，从 0 开始，必须小于会话的分片总数
	ChunkHash  string `form:"chunk_hash" binding:"required"` // 本片内容的 MD5 十六进制字符串，服务端重新计算并校验
}

// ChunkStatusRequest 查询已完成分片，帮助客户端跳过已上传部分。
type ChunkStatusRequest struct {
	UploadID string `json:"upload_id" binding:"required"` // 要查询的上传会话标识
}

// CompleteChunkUploadRequest 确认分片齐全后按序合并文件；返回播放地址，不直接发布视频。
type CompleteChunkUploadRequest struct {
	UploadID string `json:"upload_id" binding:"required"` // 要合并的上传会话标识
}
