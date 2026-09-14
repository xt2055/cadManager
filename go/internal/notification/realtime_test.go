package notification

import (
 "context"
 "errors"
 "net/http/httptest"
 "strings"
 "testing"
 "time"

 "cadguanliq/internal/auth"
 "cadguanliq/internal/dbtest"
 "cadguanliq/internal/http/middleware"
 "golang.org/x/net/websocket"
)

type testAuth struct{}
func (testAuth) CurrentUser(_ context.Context,token string)(auth.AuthUser,error){
 if token=="valid" { return auth.AuthUser{ID:"recipient"},nil }
 return auth.AuthUser{},errors.New("invalid token")
}

func TestWebSocketAuthenticationIsolationAndDisconnect(t *testing.T) {
 h:=NewHub();h.ready=true
 server:=httptest.NewServer(middleware.Logging(h.Handler(testAuth{},[]string{"https://client.example"})))
 defer server.Close()
 dial:=func(token string)*websocket.Conn{
  t.Helper()
  conn,err:=websocket.Dial("ws"+strings.TrimPrefix(server.URL,"http"),"","https://client.example")
  if err!=nil { t.Fatal(err) }
  t.Cleanup(func(){conn.Close()})
  if err=websocket.JSON.Send(conn,map[string]string{"type":"auth","token":token});err!=nil { t.Fatal(err) }
  return conn
 }
 receive:=func(conn *websocket.Conn)string{
  t.Helper();_ = conn.SetReadDeadline(time.Now().Add(2*time.Second))
  var event struct {Type string `json:"type"`}
  if err:=websocket.JSON.Receive(conn,&event);err!=nil { t.Fatal(err) }
  return event.Type
 }
 invalid:=dial("bad")
 if got:=receive(invalid);got!="unauthorized" { t.Fatal(got) }
 conn:=dial("valid")
 if got:=receive(conn);got!="ready" { t.Fatal(got) }
 // A different user's channel cannot wake this client.
 h.publish("other")
 _=conn.SetReadDeadline(time.Now().Add(40*time.Millisecond))
 var event map[string]string
 if err:=websocket.JSON.Receive(conn,&event);err==nil { t.Fatal("cross-user event leaked") }
 h.publish("recipient")
 if got:=receive(conn);got!="changed" { t.Fatal(got) }
 h.disconnect()
 _=conn.SetReadDeadline(time.Now().Add(2*time.Second))
 if err:=websocket.JSON.Receive(conn,&event);err==nil { t.Fatal("listener failure did not disconnect client") }
 if _,_,ok:=h.subscribe("recipient");ok { t.Fatal("subscribed while listener disconnected") }
 if bad,err:=websocket.Dial("ws"+strings.TrimPrefix(server.URL,"http"),"","https://untrusted.example");err==nil { bad.Close();t.Fatal("untrusted origin accepted") }
}

func TestNotificationDatabaseRealtimeCommit(t *testing.T) {
 db:=dbtest.New(t);fx:=db.Seed(t)
 h:=NewHub()
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 done:=make(chan struct{})
 go func(){defer close(done);h.Run(ctx,db.Pool)}()
 t.Cleanup(func(){cancel();select{case <-done:case <-time.After(3*time.Second):t.Error("listener did not stop")}})
 var changes chan struct{}
 deadline:=time.Now().Add(3*time.Second)
 for {
  var unsubscribe func();var ok bool
  changes,unsubscribe,ok=h.subscribe(fx.Author)
  if ok { defer unsubscribe();break }
  if time.Now().After(deadline){t.Fatal("listener not ready")}
  time.Sleep(10*time.Millisecond)
 }
 tx,err:=db.Pool.Begin(ctx);if err!=nil {t.Fatal(err)}
 defer tx.Rollback(ctx)
 if _,err=tx.Exec(ctx,`INSERT INTO notifications(recipient_id,kind,title,event_key) VALUES($1::uuid,'announcement','实时通知','socket-test')`,fx.Author);err!=nil {t.Fatal(err)}
 select{case <-changes:t.Fatal("pushed before transaction commit");case <-time.After(40*time.Millisecond):}
 if err=tx.Commit(ctx);err!=nil{t.Fatal(err)}
 select{case <-changes:case <-time.After(2*time.Second):t.Fatal("missing committed push")}
 db.Exec(t,`UPDATE notifications SET read_at=now() WHERE recipient_id=$1::uuid`,fx.Author)
 select{case <-changes:case <-time.After(2*time.Second):t.Fatal("missing read-state push")}
 tx,err=db.Pool.Begin(ctx);if err!=nil {t.Fatal(err)}
 if _,err=tx.Exec(ctx,`INSERT INTO notifications(recipient_id,kind,title,event_key) VALUES($1::uuid,'announcement','回滚通知','rollback-test')`,fx.Author);err!=nil {t.Fatal(err)}
 if err=tx.Rollback(ctx);err!=nil{t.Fatal(err)}
 select{case <-changes:t.Fatal("pushed rolled-back notification");case <-time.After(80*time.Millisecond):}
}
