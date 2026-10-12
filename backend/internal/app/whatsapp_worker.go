package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

type waTask struct{ Key, Reference, State, Due, Detail, Kind string }

func waTaskFingerprint(t waTask) string { return digest(t.State + "|" + t.Due + "|" + t.Detail) }

var waStageGrant = map[string]string{"Planning": "plan", "Development": "develop", "Sampling": "sample", "Cutting": "cut", "Sewing": "sew", "Finishing": "finish", "Packing": "pack", "Warehouse": "finance", "Invoiced": "warehouse"}

// Reuse the recipient's permission and company/team/line/PO scopes, never a global work list.
func (a *App) waTasks(ctx context.Context, u *User) ([]waTask, error) {
	ctx = context.WithValue(context.WithValue(ctx, serviceKey{}, ""), authKey{}, u)
	tasks := []waTask{}
	if u.Can("view_orders") {
		orders, e := rows(ctx, a.DB, `SELECT o.id,o.po,o.stage,o.approval,o.due_date::text AS due,o.quantity,GREATEST(0,o.quantity-COALESCE((SELECT sum(good) FROM entries e WHERE e.order_id=o.id AND e.stage=o.stage),0)) AS remaining FROM orders o WHERE `+scopeSQL+` AND o.stage<>'Dispatched' AND o.approval<>'Rejected' ORDER BY o.id`, scopeArgs(u)...)
		if e != nil {
			return nil, e
		}
		for _, o := range orders {
			state := str(o, "stage")
			if str(o, "approval") == "Pending" {
				if !u.Can("approve") {
					continue
				}
				state = "Approval required"
			} else if !u.Can(waStageGrant[state]) {
				continue
			}
			tasks = append(tasks, waTask{Key: fmt.Sprintf("order:%d", id(o, "id")), Reference: "PO " + str(o, "po"), State: state, Due: str(o, "due"), Detail: fmt.Sprintf("%d pieces pending at %s", id(o, "remaining"), state), Kind: "assignment"})
		}
	}
	if sampleRead(u) {
		items, e := rows(ctx, a.DB, `SELECT d.id,d.status,d.created_by,COALESCE(d.profile->>'buyer_style','') AS ref,COALESCE(d.profile->>'buyer_required_date','') AS due,(SELECT count(*) FROM sample_indents i WHERE i.request_id=d.id AND i.status NOT IN ('Handed over','Rejected')) AS remaining FROM sample_requests d WHERE `+sampleScope+` AND d.status<>'Draft' ORDER BY d.id`, developmentArgs(u)...)
		if e != nil {
			return nil, e
		}
		for _, s := range items {
			state := str(s, "status")
			eligible := (state == "Submitted" && sampleApprove(u)) || (state == "Approved" && sampleProduction(u) && id(s, "remaining") > 0) || (state == "Changes requested" && id(s, "created_by") == u.ID)
			if !eligible {
				continue
			}
			tasks = append(tasks, waTask{Key: fmt.Sprintf("sample:%d", id(s, "id")), Reference: fmt.Sprintf("Sample request %d %s", id(s, "id"), str(s, "ref")), State: state, Due: str(s, "due"), Detail: fmt.Sprintf("%d sample pieces pending; %s", id(s, "remaining"), state), Kind: "assignment"})
		}
	}
	if enquiryRead(u) {
		items, e := rows(ctx, a.DB, `SELECT d.id,d.status,d.created_by,COALESCE(d.profile->>'garment_delivery_date','') AS due FROM enquiries d JOIN buyers b ON b.id=d.buyer_id WHERE `+enquiryScope+` AND d.status IN ('Fabric requested','Costing','Submitted','Changes requested','Approved') ORDER BY d.id`, enquiryArgs(u)...)
		if e != nil {
			return nil, e
		}
		for _, item := range items {
			status := str(item, "status")
			eligible := status == "Fabric requested" && u.Can("enquiry_fabric") || status == "Submitted" && enquiryReview(u) || (status == "Costing" || status == "Changes requested" || status == "Approved") && enquiryWrite(u) && id(item, "created_by") == u.ID
			if eligible {
				tasks = append(tasks, waTask{Key: fmt.Sprintf("enquiry:%d", id(item, "id")), Reference: enquiryReference(item), State: status, Due: str(item, "due"), Detail: "Enquiry action pending: " + status, Kind: "assignment"})
			}
		}
	}
	if u.SetupCan("view") {
		items, e := rows(ctx, a.DB, `SELECT c.id,c.legal_name,r.id AS reminder_id,r.label,r.expires_on::text AS due FROM company_reminders r JOIN company_setups c ON c.id=r.company_id WHERE c.responsible_user_id=$1 AND c.deleted_at IS NULL AND r.active`, u.ID)
		if e != nil {
			return nil, e
		}
		for _, v := range items {
			tasks = append(tasks, waTask{Key: fmt.Sprintf("renewal:%d", id(v, "reminder_id")), Reference: str(v, "legal_name"), State: "Renewal required", Due: str(v, "due"), Detail: str(v, "label"), Kind: "renewal"})
		}
	}
	return tasks, nil
}
func (a *App) whatsappTick(parent context.Context) {
	ctx, cancel := context.WithTimeout(context.WithValue(parent, serviceKey{}, "reminders"), 50*time.Second)
	defer cancel()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return
	}
	defer tx.Rollback(ctx)
	var locked bool
	if e = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(824740)`).Scan(&locked); e != nil || !locked {
		return
	}
	if !waConfigured() {
		return
	}
	settings, e := row(ctx, tx, `SELECT * FROM whatsapp_settings WHERE id=1`)
	if e != nil {
		return
	}
	if settings["enabled"] == true {
		if e = a.waCollect(ctx, tx, id(settings, "reminder_hour")); e != nil {
			slog.Error("WhatsApp queue collection failed", "error", e)
			return
		}
	}
	// A process crash after Meta accepted a request must never trigger an automatic duplicate.
	if _, e = tx.Exec(ctx, `UPDATE whatsapp_messages SET status='Unknown',error='Send interrupted; inspect Meta before sending another test.',updated_at=now() WHERE status='Sending' AND updated_at<now()-interval '2 minutes'`); e != nil {
		return
	}
	if e = tx.Commit(ctx); e != nil {
		return
	}
	for i := 0; i < 15 && ctx.Err() == nil; i++ {
		ok := a.waDeliver(ctx)
		if !ok {
			break
		}
	}
}
func (a *App) waCollect(ctx context.Context, q queryer, hour int64) error {
	loc := time.FixedZone("IST", 19800)
	now := time.Now().In(loc)
	day := now.Format("2006-01-02")
	users, e := rows(ctx, q, `SELECT u.id,u.phone FROM users u JOIN whatsapp_recipients r ON r.user_id=u.id WHERE u.active AND u.deleted_at IS NULL AND r.enabled ORDER BY u.id`)
	if e != nil {
		return e
	}
	for _, v := range users {
		phone, e := waNormalizePhone(str(v, "phone"))
		if e != nil {
			continue
		}
		u, e := a.loadUser(ctx, id(v, "id"))
		if e != nil {
			return e
		}
		tasks, e := a.waTasks(ctx, u)
		if e != nil {
			return e
		}
		seen := []string{}
		for _, task := range tasks {
			seen = append(seen, task.Key)
			fingerprint := waTaskFingerprint(task)
			before, e := rows(ctx, q, `SELECT fingerprint,generation,active FROM whatsapp_observations WHERE user_id=$1 AND task_key=$2`, u.ID, task.Key)
			if e != nil {
				return e
			}
			generation := int64(1)
			kind := task.Kind
			changed := len(before) == 0
			if len(before) > 0 {
				generation = id(before[0], "generation")
				changed = str(before[0], "fingerprint") != fingerprint || before[0]["active"] != true
				if changed {
					generation++
					if before[0]["active"] == true && kind != "renewal" {
						kind = "update"
					}
				}
			}
			if changed {
				params := waParams(u.Name, task.Reference, task.State+"; "+task.Detail+"; due "+task.Due, a.Origin+"/"+u.Portal)
				if e = waQueue(ctx, q, fmt.Sprintf("task:%d:%s:%d", u.ID, task.Key, generation), u.ID, phone, kind, params); e != nil {
					return e
				}
				if _, e = q.Exec(ctx, `INSERT INTO whatsapp_observations(user_id,task_key,fingerprint,generation) VALUES($1,$2,$3,$4) ON CONFLICT(user_id,task_key) DO UPDATE SET fingerprint=$3,generation=$4,active=true`, u.ID, task.Key, fingerprint, generation); e != nil {
					return e
				}
			}
		}
		if _, e = q.Exec(ctx, `UPDATE whatsapp_observations SET active=false WHERE user_id=$1 AND NOT (task_key=ANY($2::text[]))`, u.ID, seen); e != nil {
			return e
		}
		if len(tasks) > 0 && now.Hour() >= int(hour) {
			late := 0
			details := []string{}
			for _, t := range tasks {
				if t.Due != "" && t.Due < day {
					late++
				}
				if len(details) < 3 {
					details = append(details, t.Reference+": "+t.Detail)
				}
			}
			params := waParams(u.Name, fmt.Sprintf("%d work items pending; %d overdue", len(tasks), late), strings.Join(details, "; "), a.Origin+"/"+u.Portal)
			if e = waQueue(ctx, q, fmt.Sprintf("daily:%d:%s", u.ID, day), u.ID, phone, "daily", params); e != nil {
				return e
			}
		}
	}
	return nil
}
func (a *App) waDeliver(ctx context.Context) bool {
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return false
	}
	defer tx.Rollback(ctx)
	// Global lock serializes budget reservation across workers; network I/O happens after commit.
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(824741)`); e != nil {
		return false
	}
	settings, e := row(ctx, tx, `SELECT * FROM whatsapp_settings WHERE id=1`)
	if e != nil {
		return false
	}
	var today int64
	if e = tx.QueryRow(ctx, `SELECT COALESCE((SELECT attempts FROM whatsapp_budget WHERE day=(now() AT TIME ZONE 'Asia/Kolkata')::date),0)`).Scan(&today); e != nil || today >= id(settings, "daily_cap") {
		return false
	}
	jobs, e := rows(ctx, tx, `SELECT m.*,t.name,t.language FROM whatsapp_messages m JOIN whatsapp_templates t ON t.kind=m.kind WHERE m.status='Queued' AND m.next_attempt<=now() AND t.status='APPROVED' AND (m.kind='test' OR $1) ORDER BY m.id LIMIT 1 FOR UPDATE OF m SKIP LOCKED`, settings["enabled"])
	if e != nil || len(jobs) == 0 {
		return false
	}
	job := jobs[0]
	var active bool
	var currentPhone string
	e = tx.QueryRow(ctx, `SELECT u.active AND u.deleted_at IS NULL AND ($2 OR COALESCE(r.enabled,false)),u.phone FROM users u LEFT JOIN whatsapp_recipients r ON r.user_id=u.id WHERE u.id=$1`, id(job, "user_id"), str(job, "kind") == "test").Scan(&active, &currentPhone)
	if e != nil {
		return false
	}
	// Recheck permissions immediately before transmission, including reassigned work.
	if str(job, "kind") != "test" && active {
		u, err := a.loadUser(ctx, id(job, "user_id"))
		if err != nil {
			return false
		}
		tasks, err := a.waTasks(ctx, u)
		if err != nil {
			return false
		}
		valid := false
		if str(job, "kind") == "daily" {
			valid = len(tasks) > 0
			late := 0
			details := []string{}
			day := time.Now().In(time.FixedZone("IST", 19800)).Format("2006-01-02")
			for _, task := range tasks {
				if task.Due != "" && task.Due < day {
					late++
				}
				if len(details) < 3 {
					details = append(details, task.Reference+": "+task.Detail)
				}
			}
			job["parameters"] = waParams(u.Name, fmt.Sprintf("%d work items pending; %d overdue", len(tasks), late), strings.Join(details, "; "), a.Origin+"/"+u.Portal)
		} else {
			for _, task := range tasks {
				prefix := fmt.Sprintf("task:%d:%s:", u.ID, task.Key)
				if strings.HasPrefix(str(job, "event_key"), prefix) {
					var generation int64
					if err := tx.QueryRow(ctx, `SELECT generation FROM whatsapp_observations WHERE user_id=$1 AND task_key=$2 AND active`, u.ID, task.Key).Scan(&generation); err != nil {
						return false
					}
					valid = str(job, "event_key") == fmt.Sprintf("%s%d", prefix, generation)
					job["parameters"] = waParams(u.Name, task.Reference, task.State+"; "+task.Detail+"; due "+task.Due, a.Origin+"/"+u.Portal)
					break
				}
			}
		}
		active = valid
	}
	normalized, _ := waNormalizePhone(currentPhone)
	if !active || (str(job, "kind") != "test" && normalized != str(job, "phone")) || time.Since(job["created_at"].(time.Time)) > 24*time.Hour {
		_, e = tx.Exec(ctx, `UPDATE whatsapp_messages SET status='Cancelled',error='Recipient disabled, phone changed, or notification expired.',updated_at=now() WHERE id=$1`, id(job, "id"))
		if e == nil {
			e = tx.Commit(ctx)
		}
		return e == nil
	}
	if _, e = tx.Exec(ctx, `INSERT INTO whatsapp_budget(day,attempts) VALUES((now() AT TIME ZONE 'Asia/Kolkata')::date,1) ON CONFLICT(day) DO UPDATE SET attempts=whatsapp_budget.attempts+1`); e != nil {
		return false
	}
	if _, e = tx.Exec(ctx, `UPDATE whatsapp_messages SET status='Sending',parameters=$2,attempts=attempts+1,updated_at=now() WHERE id=$1`, id(job, "id"), job["parameters"]); e != nil {
		return false
	}
	if e = tx.Commit(ctx); e != nil {
		return false
	}
	var params []string
	b, _ := json.Marshal(job["parameters"])
	if json.Unmarshal(b, &params) != nil || len(params) != 4 {
		a.DB.Exec(ctx, `UPDATE whatsapp_messages SET status='Failed',error='Invalid message parameters',updated_at=now() WHERE id=$1`, id(job, "id"))
		return true
	}
	components := []M{}
	for _, p := range params {
		components = append(components, M{"type": "text", "text": p})
	}
	res, status, err := waAPI(ctx, "POST", os.Getenv("WHATSAPP_PHONE_ID")+"/messages", M{"messaging_product": "whatsapp", "to": strings.TrimPrefix(str(job, "phone"), "+"), "type": "template", "template": M{"name": str(job, "name"), "language": M{"code": str(job, "language")}, "components": []M{{"type": "body", "parameters": components}}}})
	state, metaID, detail := "Accepted", "", ""
	if err != nil {
		state = "Failed"
		detail = err.Error()
		if status == 0 || status >= 500 {
			state = "Unknown"
		} else if status == 429 && id(job, "attempts") < 3 {
			state = "Queued"
		}
	} else if list, ok := res["messages"].([]any); ok && len(list) > 0 {
		m, _ := list[0].(map[string]any)
		metaID = str(m, "id")
	}
	if err == nil && metaID == "" {
		state = "Unknown"
		detail = "Meta did not return a message ID; do not retry blindly."
	}
	var mid any
	if metaID != "" {
		mid = metaID
	}
	_, e = a.DB.Exec(ctx, `UPDATE whatsapp_messages SET status=$1,meta_id=$2,error=$3,updated_at=now(),next_attempt=now()+interval '5 minutes',sent_at=CASE WHEN $1='Accepted' THEN now() END WHERE id=$4`, state, mid, detail, id(job, "id"))
	if e != nil {
		slog.Error("WhatsApp result could not be stored", "message_id", id(job, "id"))
	}
	return true
}
