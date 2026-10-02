package api

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

type sealer struct{ key *rsa.PrivateKey }

type envelope struct {
	V  int    `json:"v"`
	EK string `json:"ek"`
	IV string `json:"iv"`
	CT string `json:"ct"`
}

var (
	errNotSealed = errors.New("plain")
	errSeal      = errors.New("bad seal")
)

func newSealer() (*sealer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return &sealer{key: key}, nil
}

func (s *sealer) spki() (string, error) {
	raw, err := x509.MarshalPKIXPublicKey(&s.key.PublicKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func (s *sealer) open(raw []byte) ([]byte, error) {
	var env envelope
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil || env.V != 1 || env.EK == "" || env.IV == "" || env.CT == "" {
		return nil, errNotSealed
	}
	if dec.Decode(new(any)) != io.EOF {
		return nil, errNotSealed
	}
	ek, e1 := base64.StdEncoding.DecodeString(env.EK)
	iv, e2 := base64.StdEncoding.DecodeString(env.IV)
	ct, e3 := base64.StdEncoding.DecodeString(env.CT)
	if e1 != nil || e2 != nil || e3 != nil {
		return nil, errSeal
	}
	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, s.key, ek, nil)
	if err != nil || len(aesKey) != 32 {
		return nil, errSeal
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, errSeal
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errSeal
	}
	plain, err := gcm.Open(nil, iv, ct, nil)
	if err != nil || len(plain) > 64<<10 {
		return nil, errSeal
	}
	return plain, nil
}

func (s *sealer) seal(v any) ([]byte, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	aesKey := make([]byte, 32)
	iv := make([]byte, 12)
	if _, err = rand.Read(aesKey); err != nil {
		return nil, err
	}
	if _, err = rand.Read(iv); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	wrapped, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &s.key.PublicKey, aesKey, nil)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{V: 1, EK: base64.StdEncoding.EncodeToString(wrapped), IV: base64.StdEncoding.EncodeToString(iv), CT: base64.StdEncoding.EncodeToString(gcm.Seal(nil, iv, plain, nil))})
}

func (s *Server) sealKey(w http.ResponseWriter, r *http.Request) {
	spki, err := s.keys.spki()
	if err != nil {
		reply(w, 500, map[string]string{"error": "加密通道未就绪"})
		return
	}
	reply(w, 200, map[string]string{"alg": "RSA-OAEP-256+A256GCM", "spki": spki})
}

func (s *Server) readJSON(w http.ResponseWriter, r *http.Request, v any, requireSeal, strict bool) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		reply(w, 415, map[string]string{"error": "请求必须是 JSON"})
		return false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 96<<10))
	if err != nil {
		reply(w, 400, map[string]string{"error": "请求 JSON 或字段无效"})
		return false
	}
	plain, err := s.keys.open(raw)
	if err != nil {
		if requireSeal || !errors.Is(err, errNotSealed) {
			msg := "请求必须加密后提交"
			if errors.Is(err, errSeal) {
				msg = "加密密钥已更新，请重试"
			}
			reply(w, 400, map[string]string{"error": msg})
			return false
		}
		plain = raw
	}
	dec := json.NewDecoder(bytes.NewReader(plain))
	if strict {
		dec.DisallowUnknownFields()
	}
	if err = dec.Decode(v); err != nil {
		reply(w, 400, map[string]string{"error": "请求 JSON 或字段无效"})
		return false
	}
	if dec.Decode(new(any)) != io.EOF {
		reply(w, 400, map[string]string{"error": "只允许一个 JSON 对象"})
		return false
	}
	return true
}
