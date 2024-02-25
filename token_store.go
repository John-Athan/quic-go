package quic

import (
	b64 "encoding/base64"
	"fmt"
	"sync"

	list "github.com/quic-go/quic-go/internal/utils/linkedlist"
)

type singleOriginTokenStore struct {
	tokens []*ClientToken
	len    int
	p      int
}

func newSingleOriginTokenStore(size int) *singleOriginTokenStore {
	return &singleOriginTokenStore{tokens: make([]*ClientToken, size)}
}

func (s *singleOriginTokenStore) Add(token *ClientToken) {
	//if s.len == 0 {
	s.tokens[s.p] = token
	//s.p = s.index(s.p + 1)
	//s.len = utils.Min(s.len+1, len(s.tokens))
	//}
}

func (s *singleOriginTokenStore) Pop() *ClientToken {
	// Right now, we don't want to actually pop the token, just retrieve it
	//s.p = s.index(s.p - 1)
	token := s.tokens[s.p]
	//s.tokens[s.p] = nil
	//s.len = utils.Max(s.len-1, 0)
	return token
}

func (s *singleOriginTokenStore) Len() int {
	return s.len
}

func (s *singleOriginTokenStore) index(i int) int {
	mod := len(s.tokens)
	return (i + mod) % mod
}

type lruTokenStoreEntry struct {
	key         string
	cache       *singleOriginTokenStore
	hasBeenUsed bool
}

type lruTokenStore struct {
	mutex sync.Mutex

	m                map[string]*list.Element[*lruTokenStoreEntry]
	q                *list.List[*lruTokenStoreEntry]
	capacity         int
	singleOriginSize int
}

var _ TokenStore = &lruTokenStore{}

// NewLRUTokenStore creates a new LRU cache for tokens received by the client.
// maxOrigins specifies how many origins this cache is saving tokens for.
// tokensPerOrigin specifies the maximum number of tokens per origin.
func NewLRUTokenStore(maxOrigins, tokensPerOrigin int) TokenStore {
	return &lruTokenStore{
		m:                make(map[string]*list.Element[*lruTokenStoreEntry]),
		q:                list.New[*lruTokenStoreEntry](),
		capacity:         maxOrigins,
		singleOriginSize: tokensPerOrigin,
	}
}

func (s *lruTokenStore) PutString(key string, tokenString []byte) {
	token := &ClientToken{
		data: tokenString,
	}
	s.Put(key, token)
}

func (s *lruTokenStore) Put(key string, token *ClientToken) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if el, ok := s.m[key]; ok {
		entry := el.Value
		entry.cache.Add(token)
		s.q.MoveToFront(el)
		return
	}

	if s.q.Len() < s.capacity {
		entry := &lruTokenStoreEntry{
			key:         key,
			cache:       newSingleOriginTokenStore(s.singleOriginSize),
			hasBeenUsed: false,
		}
		entry.cache.Add(token)
		s.m[key] = s.q.PushFront(entry)
		return
	}

	elem := s.q.Back()
	entry := elem.Value
	delete(s.m, entry.key)
	entry.key = key
	entry.cache = newSingleOriginTokenStore(s.singleOriginSize)
	entry.cache.Add(token)
	s.q.MoveToFront(elem)
	s.m[key] = elem
}

func (s *lruTokenStore) Pop(key string) *ClientToken {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var token *ClientToken
	if el, ok := s.m[key]; ok {
		s.q.MoveToFront(el)
		cache := el.Value.cache
		s.m[key].Value.hasBeenUsed = true
		token = cache.Pop()
		if cache.Len() == 0 {
			//s.q.Remove(el)
			//delete(s.m, key)
		}
	}
	return token
}

func (s *lruTokenStore) GetToken(key string) string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	v := s.m[key]
	if v != nil && v.Value != nil && v.Value.cache != nil && v.Value.cache.tokens != nil && len(v.Value.cache.tokens) > 0 {
		if v.Value.cache.tokens[0] == nil {
			print("asd")
		}
		return b64.StdEncoding.EncodeToString(v.Value.cache.tokens[0].data)
	}
	return ""
}

func (s *lruTokenStore) Size(key string) int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for k, v := range s.m {
		if k == key && len(v.Value.cache.tokens) > 0 {
			return len(v.Value.cache.tokens[0].data)
		}
	}
	return 0
}

func (s *lruTokenStore) HasBeenUsed(key string) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for k, v := range s.m {
		if k == key && v.Value.hasBeenUsed {
			v.Value.hasBeenUsed = false
			return true
		}
	}
	return false
}

func (s *lruTokenStore) Describe() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	ret := "The token store contains the following items:\n"
	for k, v := range s.m {
		ret = ret + fmt.Sprintf("Domain: %s Used: %t Token: %s\n", k, v.Value.hasBeenUsed, b64.StdEncoding.EncodeToString(v.Value.cache.tokens[0].data))
	}
	return ret
}
