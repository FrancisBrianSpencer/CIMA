package main

import (
 "context"
 "encoding/json"
 "log"
 "net/http"
 "os"
 "time"

 "github.com/go-chi/chi/v5"
 "go.mongodb.org/mongo-driver/bson"
 "go.mongodb.org/mongo-driver/mongo"
 "go.mongodb.org/mongo-driver/mongo/options"
)

type App struct{ db *mongo.Database }
type Resident struct { ID interface{} `json:"id,omitempty" bson:"_id,omitempty"`; FirstName string `json:"firstName" bson:"firstName"`; LastName string `json:"lastName" bson:"lastName"`; Status string `json:"status" bson:"status"` }

func main() {
 ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second); defer cancel()
 client, err := mongo.Connect(ctx, options.Client().ApplyURI(getenv("MONGO_URI", "mongodb://mongodb:27017"))); if err != nil { log.Fatal(err) }
 if err = client.Ping(ctx, nil); err != nil { log.Fatal(err) }
 app := &App{db: client.Database(getenv("MONGO_DATABASE", "cima"))}
 r := chi.NewRouter(); r.Get("/health", app.health)
 r.Route("/api/v1", func(r chi.Router) { r.Get("/residents", app.listResidents); r.Post("/residents", app.createResident) })
 log.Println("CIMA API listening on :8080"); log.Fatal(http.ListenAndServe(":"+getenv("APP_PORT", "8080"), r))
}
func (a *App) health(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status":"ok","service":"cima-api"}) }
func (a *App) listResidents(w http.ResponseWriter, r *http.Request) { ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second); defer cancel(); cur, err := a.db.Collection("residents").Find(ctx, bson.M{}); if err != nil { http.Error(w,"database error",500); return }; defer cur.Close(ctx); out:=[]Resident{}; if err=cur.All(ctx,&out); err != nil { http.Error(w,"database error",500); return }; writeJSON(w,200,out) }
func (a *App) createResident(w http.ResponseWriter, r *http.Request) { var in Resident; if json.NewDecoder(r.Body).Decode(&in)!=nil || in.FirstName=="" || in.LastName=="" { writeJSON(w,400,map[string]string{"error":"firstName and lastName are required"}); return }; if in.Status=="" { in.Status="active" }; ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second); defer cancel(); result,err:=a.db.Collection("residents").InsertOne(ctx,in); if err!=nil { http.Error(w,"database error",500); return }; in.ID=result.InsertedID; writeJSON(w,201,in) }
func writeJSON(w http.ResponseWriter, status int, v interface{}) { w.Header().Set("Content-Type","application/json"); w.WriteHeader(status); _=json.NewEncoder(w).Encode(v) }
func getenv(k, fallback string) string { if v:=os.Getenv(k); v!="" { return v }; return fallback }
