package db

import (
	"errors"
	"strings"

	"github.com/afterglow-labs/afterglow-cybersecurity/chatgptllm"
)

// saveChatGPTProfile keeps OAuth credentials out of PostgreSQL. A plan profile
// only contains an account identifier; inference obtains a token from the local
// credential manager on each request. The credential scope changes atomically.
func (d *DB) saveChatGPTProfile(p *LLMProfile) (int64, error) {
	if strings.TrimSpace(p.ChatGPTAccountID) == "" || strings.TrimSpace(p.Model) == "" {
		return 0, errors.New("a connected ChatGPT account and model are required")
	}
	if p.APIKey != "" {
		return 0, errors.New("ChatGPT plan profiles cannot contain an API key")
	}
	if err := chatgptllm.ValidateOptions(p.Model, "chatgpt_plan", p.ReasoningEffort, p.AccessProgramCyber); err != nil {
		return 0, err
	}
	p.Format = "openai-responses"
	p.BaseURL, p.APIKeyHint, p.MaxTokensField, p.SessionHeaderKey = "", "", "", ""
	p.MaxTokens = 0
	p.Streaming = true
	p.PoolExclude = true
	r := p.Retry.Clamped()
	if p.ID == 0 {
		var id int64
		err := d.QueryRow(`INSERT INTO llm_profiles (
name,format,base_url,proxy,model,api_key,api_key_hint,auth_mode,chatgpt_account_id,access_program_cyber,
rate_per_second,rate_per_minute,context_window_k,reasoning_effort,priority,pool_exclude,thinking_type,streaming,
max_tokens,max_tokens_field,session_header_key,retry_connect_attempts,retry_connect_interval_ms,
retry_empty_attempts,retry_empty_interval_ms,retry_stream_attempts,retry_stream_interval_ms,keep_alive,llm_max_concurrent)
VALUES ($1,'openai-responses',NULL,NULLIF($2,''),$3,NULL,NULL,'chatgpt_plan',$4,$5,
$6,$7,$8,$9,$10,true,$11,true,0,'','',$12,$13,$14,$15,$16,$17,$18,$19) RETURNING id`,
			p.Name, p.Proxy, p.Model, p.ChatGPTAccountID, p.AccessProgramCyber, p.RatePerSecond, p.RatePerMinute,
			p.ContextWindowK, p.ReasoningEffort, p.Priority, p.ThinkingType, r.Connect.Attempts, r.Connect.IntervalMS,
			r.Empty.Attempts, r.Empty.IntervalMS, r.Stream.Attempts, r.Stream.IntervalMS, p.KeepAlive, p.MaxConcurrent).Scan(&id)
		return id, err
	}
	res, err := d.Exec(`UPDATE llm_profiles SET name=$1,format='openai-responses',base_url=NULL,proxy=NULLIF($2,''),model=$3,
api_key=NULL,api_key_hint=NULL,auth_mode='chatgpt_plan',chatgpt_account_id=$4,access_program_cyber=$5,
rate_per_second=$6,rate_per_minute=$7,context_window_k=$8,reasoning_effort=$9,priority=$10,pool_exclude=true,
thinking_type=$11,streaming=true,max_tokens=0,max_tokens_field='',session_header_key='',
retry_connect_attempts=$12,retry_connect_interval_ms=$13,retry_empty_attempts=$14,retry_empty_interval_ms=$15,
retry_stream_attempts=$16,retry_stream_interval_ms=$17,keep_alive=$18,llm_max_concurrent=$19 WHERE id=$20`,
		p.Name, p.Proxy, p.Model, p.ChatGPTAccountID, p.AccessProgramCyber, p.RatePerSecond, p.RatePerMinute,
		p.ContextWindowK, p.ReasoningEffort, p.Priority, p.ThinkingType, r.Connect.Attempts, r.Connect.IntervalMS,
		r.Empty.Attempts, r.Empty.IntervalMS, r.Stream.Attempts, r.Stream.IntervalMS, p.KeepAlive, p.MaxConcurrent, p.ID)
	if err != nil {
		return p.ID, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return p.ID, err
	}
	if n == 0 {
		return p.ID, ErrLLMProfileNotFound
	}
	return p.ID, nil
}
