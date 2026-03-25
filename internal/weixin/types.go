package weixin

const (
	messageTypeUser = 1
	messageTypeBot  = 2

	messageItemText  = 1
	messageItemImage = 2
	messageItemVoice = 3
	messageItemFile  = 4
	messageItemVideo = 5

	messageStateFinish = 2

	sessionExpiredErrcode = -14
	uploadMediaImage      = 1
)

type baseInfo struct {
	ChannelVersion string `json:"channel_version,omitempty"`
}

type getUpdatesReq struct {
	GetUpdatesBuf string   `json:"get_updates_buf"`
	BaseInfo      baseInfo `json:"base_info"`
}

type getUpdatesResp struct {
	Ret                  int             `json:"ret"`
	Errcode              int             `json:"errcode"`
	Errmsg               string          `json:"errmsg"`
	Msgs                 []weixinMessage `json:"msgs"`
	GetUpdatesBuf        string          `json:"get_updates_buf"`
	LongpollingTimeoutMs int             `json:"longpolling_timeout_ms"`
}

type textItem struct {
	Text string `json:"text,omitempty"`
}

type cdnMedia struct {
	EncryptQueryParam string `json:"encrypt_query_param,omitempty"`
	AESKey            string `json:"aes_key,omitempty"`
	EncryptType       int    `json:"encrypt_type,omitempty"`
}

type imageItem struct {
	Media      *cdnMedia `json:"media,omitempty"`
	ThumbMedia *cdnMedia `json:"thumb_media,omitempty"`
	AESKeyHex  string    `json:"aeskey,omitempty"`
	MidSize    int       `json:"mid_size,omitempty"`
}

type voiceItem struct {
	Text string `json:"text,omitempty"`
}

type refMessage struct {
	MessageItem *messageItem `json:"message_item,omitempty"`
	Title       string       `json:"title,omitempty"`
}

type messageItem struct {
	Type      int         `json:"type,omitempty"`
	TextItem  *textItem   `json:"text_item,omitempty"`
	ImageItem *imageItem  `json:"image_item,omitempty"`
	VoiceItem *voiceItem  `json:"voice_item,omitempty"`
	RefMsg    *refMessage `json:"ref_msg,omitempty"`
}

type getUploadURLRequest struct {
	Filekey     string   `json:"filekey,omitempty"`
	MediaType   int      `json:"media_type,omitempty"`
	ToUserID    string   `json:"to_user_id,omitempty"`
	Rawsize     int      `json:"rawsize,omitempty"`
	Rawfilemd5  string   `json:"rawfilemd5,omitempty"`
	Filesize    int      `json:"filesize,omitempty"`
	NoNeedThumb bool     `json:"no_need_thumb,omitempty"`
	Aeskey      string   `json:"aeskey,omitempty"`
	BaseInfo    baseInfo `json:"base_info"`
}

type getUploadURLResponse struct {
	UploadParam string `json:"upload_param,omitempty"`
}

type weixinMessage struct {
	Seq          int64         `json:"seq,omitempty"`
	MessageID    int64         `json:"message_id,omitempty"`
	FromUserID   string        `json:"from_user_id,omitempty"`
	ToUserID     string        `json:"to_user_id,omitempty"`
	ClientID     string        `json:"client_id,omitempty"`
	CreateTimeMs int64         `json:"create_time_ms,omitempty"`
	MessageType  int           `json:"message_type,omitempty"`
	ItemList     []messageItem `json:"item_list,omitempty"`
	ContextToken string        `json:"context_token,omitempty"`
}

type sendMessageReq struct {
	Msg      weixinOutboundMsg `json:"msg"`
	BaseInfo baseInfo          `json:"base_info"`
}

type sendMessageResp struct {
	Ret     int    `json:"ret"`
	Errcode int    `json:"errcode"`
	Errmsg  string `json:"errmsg"`
}

type weixinOutboundMsg struct {
	FromUserID   string        `json:"from_user_id"`
	ToUserID     string        `json:"to_user_id"`
	ClientID     string        `json:"client_id"`
	MessageType  int           `json:"message_type"`
	MessageState int           `json:"message_state"`
	ItemList     []messageItem `json:"item_list,omitempty"`
	ContextToken string        `json:"context_token,omitempty"`
}
