// session manager

package session

import (
    "fmt"
    "crypto/rand"
    "sync"
    "io"
    "encoding/base64"
    "net/http"
    "net/url"
    log "github.com/cihub/seelog"        
)
    


type Provider interface {
    SessionInit(sid string) (Session, error)
    SessionRead(sid string) (Session, error)
    SessionDestroy(sid string) error
    SessionGC(maxLifeTime int64)
}

type Session interface {
    Set(key, value interface{}) error //set session value
    Get(key interface{}) interface{}  //get session value
    Delete(key interface{}) error     //delete session value
    SessionID() string                //back current sessionID
}


var provides = make(map[string]Provider)

//register make a sesscion provider available by the provided name
func Register(name string, provider Provider) {
    if provider == nil {
        panic("session: register provider is nil")
    }

    if _, dup := provides[name]; dup {
        panic("session: Register called twice for provider" + name)
    }

    provides[name] = provider
}


type Manager struct {
    cookieName string  // private cookie name
    lock sync.Mutex
    provider Provider 
    maxlifetime int64
}

func NewManager(provideName string, cookieName string, maxlifetime int64) (*Manager, error) {
    provider, ok := provides[provideName]
    log.Info("new session manager")
    if !ok {
        log.Error("no valid provider ,error")
        return nil, fmt.Errorf("session: unknown provide %q (forgotten import?)", provideName)
    }
    return &Manager{provider: provider, cookieName: cookieName, maxlifetime: maxlifetime}, nil
}

// get unique global session id
func (manager *Manager) sessionId() string {
    b := make([]byte, 32)
    if _, err := io.ReadFull(rand.Reader, b); err != nil {
        return ""
    }
    return base64.URLEncoding.EncodeToString(b)
}

func (manager *Manager) SessionStart(w http.ResponseWriter, r *http.Request) (session Session) {
    manager.lock.Lock()
    defer manager.lock.Unlock()
    cookie, err := r.Cookie(manager.cookieName)
    if err != nil || cookie.Value == "" {
        log.Debug("no valid session id in request cookie, create one")
        sid := manager.sessionId()
        log.Debug("new created sid is ", sid)
        session, _ = manager.provider.SessionInit(sid)
        cookie := http.Cookie{Name: manager.cookieName, Value: url.QueryEscape(sid), Path: "/", HttpOnly: true, MaxAge: int(manager.maxlifetime)}
        http.SetCookie(w, &cookie)
    } else {
        sid, _ := url.QueryUnescape(cookie.Value)
        log.Debugf("get valid session id  %s in request cookie %s\n", sid, manager.cookieName)        
        session, _ = manager.provider.SessionRead(sid)
    }

    return session
}


func (manager *Manager) SessionEnd(w http.ResponseWriter, s Session) {
    manager.lock.Lock()
    defer manager.lock.Unlock()
    sid := s.SessionID()
    // delete cookie now, set max age to < 0 value
    cookie := http.Cookie{Name: manager.cookieName, Value: url.QueryEscape(sid), Path: "/", HttpOnly: true, MaxAge: -1}
    http.SetCookie(w, &cookie)

    log.Debugf("destroy session for id %s \n", sid) 
    err := manager.provider.SessionDestroy(sid)
    if err != nil {
        log.Errorf("destroy session for id %s failed\n", sid)
    }
}


// start session for json api
func (manager *Manager) ApiSessionStart(r *http.Request) (session Session) {

    sid := r.Header.Get("X-Session-Token")
    log.Debugf("get session token is %s", sid)
    sid, _ = url.QueryUnescape(sid)        


    if sid == "" {
        log.Debug("no valid session id in request, create one")
        session = manager.ApiSessionCreate()
    } else {
        manager.lock.Lock()
        defer manager.lock.Unlock()        
        //log.Debugf("get valid session id  %s", sid)        
        session, _ = manager.provider.SessionRead(sid)
    }
    return session
}

func (manager *Manager) ApiSessionCreate() (session Session) {
    manager.lock.Lock()
    defer manager.lock.Unlock()

    sid := manager.sessionId()
    log.Debug("new created sid is ", sid)
    session, _ = manager.provider.SessionInit(sid)
    return session
}


// end session for json api
func (manager *Manager) ApiSessionEnd(session Session) {

    manager.lock.Lock()
    defer manager.lock.Unlock()
    sid := session.SessionID()

    log.Debugf("destroy session for id %s \n", sid) 
    err := manager.provider.SessionDestroy(sid)
    if err != nil {
        log.Errorf("destroy session for id %s failed\n", sid)
    }
}
