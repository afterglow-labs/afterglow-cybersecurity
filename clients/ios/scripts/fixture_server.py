#!/usr/bin/env python3
"""Loopback-only synthetic APTEX API. No models, production credentials, or jobs."""
import json
import threading
import time
import uuid
from http.cookies import SimpleCookie
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlsplit

LOCK = threading.RLock()
STATE = {}

def reset():
    with LOCK:
        STATE.clear()
        STATE.update(sessions={}, cursor=2, revision=1, history=1, requests=[], inputs={}, uploads=0,
                     tasks=[dict(id='42', name='Local fixture assessment', description='Inspect only the synthetic fixture.',
                                 goal='Review fixture controls', status='running', goals_total=2, goals_met=1)],
                     conversations=[dict(id=7, agent_key='mainagent', title='Fixture conversation', llm_profile_id=None, running=False)],
                     activities={7: [dict(seq=1, worker='user', kind='text', summary='Synthetic fixture prompt'),
                                     dict(seq=2, worker='assistant', kind='text', summary='Synthetic fixture response', detail='Synthetic fixture response\n\nFull message detail is loaded automatically.')]})
reset()

def controls():
    return dict(revision=str(STATE['revision']), history_revision=str(STATE['history']), paused=STATE.get('paused', False),
                context_changing=False, active=None, inputs=[])

FINDING = dict(id='3', name='Fixture finding', vulnclass='configuration', severity='medium', status='confirmed',
               summary='Synthetic evidence only.', evidence='Fixture evidence retained.', report='Complete fixture report.', task_id='42')

class Handler(BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'
    def log_message(self, *args):
        pass
    def token(self):
        bearer = self.headers.get('Authorization', '')
        if bearer.startswith('Bearer '): return bearer[7:]
        cookie = SimpleCookie(self.headers.get('Cookie', ''))
        return cookie['afterglow_token'].value if 'afterglow_token' in cookie else ''
    def authorized(self):
        with LOCK: return STATE['sessions'].get(self.token(), False)
    def send(self, data, status=200, headers=None, content_type='application/json'):
        raw = data if isinstance(data, bytes) else json.dumps(data).encode()
        self.send_response(status)
        self.send_header('Content-Type', content_type)
        self.send_header('Content-Length', str(len(raw)))
        self.send_header('Cache-Control', 'no-store')
        for key, value in (headers or {}).items(): self.send_header(key, value)
        self.end_headers()
        self.wfile.write(raw)
    def do_GET(self):
        path = urlsplit(self.path).path
        with LOCK: STATE['requests'].append(dict(path=path, cursor=self.headers.get('Last-Event-ID')))
        if path == '/_fixture/stats':
            with LOCK: self.send(dict(active_sessions=sum(STATE['sessions'].values()), admissions=len(STATE['inputs']), uploads=STATE['uploads'], requests=STATE['requests']))
            return
        if path == '/api/v1/meta': self.send(dict(service='aptex', api_version='1', server_version='ios-fixture')); return
        if path == '/api/ready': self.send(dict(ready=True)); return
        if path.startswith('/api/') and not self.authorized(): self.send({'error':'Invalid or expired session'},401); return
        if path in ['/api/v1/auth/session', '/api/auth/status']:
            self.send(dict(user={'username':'Afterglow'},expires_at='2099-01-01T00:00:00Z',api_version='1',initialized=True,username='Afterglow')); return
        if path == '/api/v1/agents': self.send(dict(agents=[dict(id='mainagent', key='mainagent', name='Main agent',enabled=True,role='mainagent')])); return
        if path == '/api/v1/llm/profiles':
            self.send(dict(profiles=[dict(id='9007199254741011',name='SOL fixture',model='fixture-sol',auth_mode='api_key',connected=True,access_program_cyber='blue'),dict(id='2',name='Astra fixture',model='fixture-astra',auth_mode='api_key',connected=True,access_program_cyber='')])); return
        if path == '/api/v1/fixture-report': self.send(b'Synthetic report export',headers={'Content-Disposition':'attachment; filename="fixture-report.txt"'},content_type='text/plain'); return
        if path == '/api/v1/tasks':
            with LOCK: self.send(dict(tasks=STATE['tasks'],active='42' if STATE['tasks'][0]['status']=='running' else ''))
            return
        if path.startswith('/api/v1/tasks/') and path.rsplit('/',1)[1].isdigit():
            with LOCK: self.send(next(t for t in STATE['tasks'] if t['id']==path.rsplit('/',1)[1]))
            return
        if path.endswith('/goals'): self.send(dict(goals=[dict(id='1',text='Verify fixture boundary',state='met')])); return
        if path.endswith('/constraints'): self.send(dict(constraints=[dict(id='1',kind='allow',text='Synthetic fixture only')])); return
        if path == '/api/v1/conversations':
            with LOCK: self.send(dict(conversations=STATE['conversations']))
            return
        if path.startswith('/api/v1/conversations/'):
            try: cid=int(path.split('/')[4])
            except ValueError: self.send({},404); return
            if path.endswith('/events'): self.events(cid); return
            if path.endswith('/controls'):
                with LOCK: self.send(controls())
                return
            if '/messages' in path:
                with LOCK:
                    items=STATE['activities'].get(cid,[])
                    if path.endswith('/messages'):
                        self.send(dict(items=[{k:v for k,v in i.items() if k!="detail"} for i in items],cursor=max([i['seq'] for i in items],default=0),running=False,hasMore=False))
                    else:
                        seq=int(path.rsplit('/',1)[1]); self.send(dict(detail=next((i.get('detail',i.get('summary','')) for i in items if i['seq']==seq),'')))
                return
        if path == '/api/v1/exploration/findings': self.send(dict(items=[FINDING],total=1,page=1,page_size=100)); return
        if path == '/api/v1/exploration/findings/3': self.send(FINDING); return
        if not path.startswith('/api/'):
            if not self.authorized():
                self.send(b'',307,{'Location':'/login'}); return
            html=b'<!doctype html><html lang="en"><meta name="viewport" content="width=device-width,initial-scale=1"><body style="background:#091117;color:#67e8f9;font:16px monospace;padding:24px"><h1>APTEX Console</h1><p>Console fixture ready</p><p>Graphs, workflows, traffic, interception and model settings</p><input placeholder="Console draft" aria-label="Console draft"><a href="/api/v1/fixture-report" download>Download fixture report</a><p id="export-status">Export idle</p><button onclick="document.getElementById(\'export-status\').textContent=\'Export started\';try {var a=document.createElement(\'a\');a.href=window.URL.createObjectURL(new Blob([\'Synthetic blob report\'],{type:\'text/plain\'}));a.download=\'fixture-blob.txt\';document.body.appendChild(a);a.click();a.remove();document.getElementById(\'export-status\').textContent=\'Export dispatched\'}catch(e){document.getElementById(\'export-status\').textContent=String(e)}">Export fixture blob</button><a href="/function/tasks">Tasks</a></body></html>'
            self.send(html,content_type='text/html'); return
        self.send({'error':'Fixture route missing'},404)
    def do_POST(self):
        path=urlsplit(self.path).path
        raw=self.rfile.read(int(self.headers.get('Content-Length','0')))
        if path=='/_fixture/reset': reset(); self.send({'ok':True}); return
        if path=='/_fixture/rewrite':
            with LOCK:
                STATE['activities'][7][1]['detail']='Updated full message after history revision.'
                STATE['history']+=1;STATE['revision']+=1
            self.send({'ok':True});return
        if path=='/api/v1/auth/login':
            data=json.loads(raw or b'{}')
            if data.get('username')!='Afterglow' or data.get('password')!='fixture-aptex-1234': self.send({'error':'Invalid credentials'},401); return
            token='fixture.'+uuid.uuid4().hex
            with LOCK: STATE['sessions'][token]=True
            self.send(dict(token=token,username='Afterglow',expires_at='2099-01-01T00:00:00Z')); return
        if not self.authorized(): self.send({'error':'Invalid or expired session'},401); return
        if path in ['/api/v1/auth/restore','/api/v1/auth/logout']:
            token=self.token()
            if path.endswith('/logout'):
                with LOCK: STATE['sessions'][token]=False
                self.send({'ok':True},headers={'Set-Cookie':'afterglow_token=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax'}); return
            self.send(dict(user={'username':'Afterglow'},expires_at='2099-01-01T00:00:00Z',api_version='1'),headers={'Set-Cookie':f'afterglow_token={token}; Path=/; HttpOnly; SameSite=Lax'}); return
        if path=='/api/v1/chat/upload':
            with LOCK: STATE['uploads']+=1
            self.send(dict(attachments=[dict(name='fixture.txt',path='attachments/fixture.txt',size=len(raw),abs='/fixture/attachment.txt')])); return
        data=json.loads(raw or b'{}')
        if path=='/api/v1/tasks':
            with LOCK:
                task=dict(id=str(len(STATE['tasks'])+42),name=data.get('name',''),description=data['description'],goal=data['goal'],status='running',goals_total=0,goals_met=0)
                STATE['tasks'].append(task)
            self.send(task); return
        if path.endswith('/control'):
            with LOCK: STATE['tasks'][0]['status']='paused' if data['action']=='pause' else 'running'
            self.send({'ok':True}); return
        if path=='/api/v1/conversations':
            with LOCK:
                cid=len(STATE['conversations'])+7
                conversation=dict(id=cid,agent_key=data['agent_key'],title=data.get('title') or 'New conversation',llm_profile_id=data.get('llm_profile_id'),running=False)
                STATE['conversations'].append(conversation); STATE['activities'][cid]=[]
            self.send(conversation); return
        if path.endswith('/controls/inputs'):
            cid=int(path.split('/')[4]); request_id=data['client_request_id']
            with LOCK:
                if request_id not in STATE['inputs']:
                    STATE['inputs'][request_id]=dict(id=request_id,status='completed',prompt={'text':data['message'],'attachments':data['attachments']},revision='1')
                    for worker,message in [('user',data['message']),('assistant','Fixture reply: '+data['message'])]:
                        STATE['cursor']+=1
                        STATE['activities'].setdefault(cid,[]).append(dict(seq=STATE['cursor'],worker=worker,kind='text',summary=message))
                    STATE['revision']+=1
                self.send(STATE['inputs'][request_id],202)
            return
        if path.endswith('/controls/pause') or path.endswith('/controls/resume'):
            with LOCK:
                if data['expected_revision']!=str(STATE['revision']): self.send({'error':'Chat state changed'},409); return
                STATE['paused']=path.endswith('/pause'); STATE['revision']+=1
                self.send(controls())
            return
        self.send({'error':'Fixture route missing'},404)
    def do_PATCH(self):
        raw=self.rfile.read(int(self.headers.get('Content-Length','0')))
        if not self.authorized(): self.send({},401); return
        self.send({'ok':True})
    def events(self,cid):
        cursor=int(self.headers.get('Last-Event-ID','0'))
        self.send_response(200); self.send_header('Content-Type','text/event-stream'); self.send_header('Cache-Control','no-store'); self.end_headers()
        try:
            while self.authorized():
                with LOCK:
                    items=[i for i in STATE['activities'].get(cid,[]) if i['seq']>cursor]
                    state=dict(cursor=str(cursor),running=False,paused=STATE.get('paused',False),history_revision=str(STATE['history']),control_revision=str(STATE['revision']),context_changing=False)
                for item in items:
                    self.wfile.write(('id: '+str(item['seq'])+'\nevent: activity\ndata: '+json.dumps({k:v for k,v in item.items() if k!='detail'})+'\n\n').encode());cursor=item['seq']
                self.wfile.write(('event: state\ndata: '+json.dumps(state)+'\n\n').encode());self.wfile.flush()
                time.sleep(.25)
            self.wfile.write(b'event: session_expired\ndata: {"reauthenticate":true}\n\n'); self.wfile.flush()
        except (BrokenPipeError,ConnectionResetError): pass

if __name__=='__main__':
    server=ThreadingHTTPServer(('127.0.0.1',18768),Handler)
    server.daemon_threads=True
    print('Synthetic APTEX fixture listening on loopback:18768',flush=True)
    server.serve_forever()
