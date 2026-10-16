import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Archive, ChevronDown, Code2, Download, FileText,
  HelpCircle, Inbox, Paperclip, Printer, RefreshCw, Reply, ReplyAll, Forward,
  Search, Send, Settings, Share, ShieldCheck, SlidersHorizontal, Star,
  Trash2, Ban, ExternalLink, Plus,
} from 'lucide-react';
import { mockEmails } from './mock';
import DOMPurify from 'dompurify';
import { AddAccount, DetectSettings, FetchMail, FetchOlder, GetMessage, ListAccounts, ListCached, ListFolders, OAuthConfigured, OAuthFinish, OAuthStart, Reconcile, RefreshAll, RemoveAccount, SendMail, StartBackfill, StartWatch, StopWatch, StorageInfo } from '../wailsjs/go/main/App';
import { EventsOff, EventsOn } from '../wailsjs/runtime/runtime';
import { BrowserOpenURL } from '../wailsjs/runtime/runtime';

const inWails = () => typeof (window as any).go !== 'undefined';

const PURPLE = '#7c3aed';

function Logo() {
  return (
    <div className="flex items-center gap-2 font-semibold text-[17px] tracking-tight">
      <div className="w-6 h-6 rounded-md flex items-center justify-center text-white" style={{ background: PURPLE }}>
        <Send size={13} strokeWidth={2.5} />
      </div>
      <span>space<span className="font-bold">mail</span></span>
    </div>
  );
}

export default function App() {
  const [activeFolder, setActiveFolder] = useState<string>('inbox');
  const [selectedId, setSelectedId] = useState<string>('3');
  const [query, setQuery] = useState('');
  const [composeOpen, setComposeOpen] = useState(false);
  const [accountOpen, setAccountOpen] = useState(false);

  // account form (autoconfig: only email+password, no SMTP ports)
  const [accName, setAccName] = useState('');
  const [accEmail, setAccEmail] = useState('');
  const [accPass, setAccPass] = useState('');
  const [accStatus, setAccStatus] = useState('');
  const [oauthID, setOauthID] = useState(() => localStorage.getItem('emailclient:oauthID') || '');
  const [oauthReady, setOauthReady] = useState<Record<string, boolean>>({});
  const [showOauthSetup, setShowOauthSetup] = useState(false);

  // when the add-account modal opens, learn which providers are pre-configured
  useEffect(() => {
    if (!accountOpen || !inWails()) return;
    Promise.all([OAuthConfigured('google'), OAuthConfigured('microsoft')])
      .then(([g, m]) => setOauthReady({ google: !!g, microsoft: !!m }))
      .catch(() => {});
  }, [accountOpen]);

  const oauthSignIn = async (provider: string) => {
    if (!inWails()) { setAccStatus('OAuth needs the desktop app — run via wails dev.'); return; }
    localStorage.setItem('emailclient:oauthID', oauthID.trim());
    try {
      const url = await OAuthStart(provider, oauthID.trim());
      if (url.startsWith('error:')) { setAccStatus(url); return; }
      BrowserOpenURL(url);
      setAccStatus('Browser opened — approve, then wait…');
      const res = await OAuthFinish();
      setAccStatus(res);
      if (!res.startsWith('error:')) {
        await refreshAccounts();
        const m = res.match(/Connected (\S+)/);
        if (m) {
          setActiveEmail(m[1].toLowerCase());
          const mails = await FetchMail(m[1].toLowerCase(), 'inbox').catch(() => null);
          if (mails) setServerMails(mails);
          loadCounts(m[1].toLowerCase());
        }
      }
    } catch (e: any) {
      setAccStatus('Failed: ' + (e?.message || e));
    }
  };

  // live backend state (falls back to mock when not in Wails / no accounts)
  const [accounts, setAccounts] = useState<Array<{ email: string }>>([]);
  const [accountsLoaded, setAccountsLoaded] = useState(false);
  const [activeEmail, setActiveEmail] = useState('');
  const [serverMails, setServerMails] = useState<any[]>([]);
  const [folderCounts, setFolderCounts] = useState<Record<string, number>>({});
  const [folderTotals, setFolderTotals] = useState<Record<string, number>>({});
  const [loadingMore, setLoadingMore] = useState(false);
  const [exhausted, setExhausted] = useState(false);
  const activeFolderRef = useRef(activeFolder);
  useEffect(() => { activeFolderRef.current = activeFolder; }, [activeFolder]);

  const loadOlder = async () => {
    if (!inWails() || !activeEmail || loadingMore || exhausted || !hasMore || serverMails.length === 0) return;
    setLoadingMore(true);
    try {
      const folder = activeFolder;
      const before = serverMails.length;
      const minUid = Math.min(...serverMails.map((m: any) => m.uid).filter((u: any) => u > 0));
      if (!isFinite(minUid)) return;
      const mails = await FetchOlder(activeEmail, folder, minUid);
      if (activeFolderRef.current !== folder) return;
      if (!mails || mails.length <= before) {
        setExhausted(true); // server has nothing new to give
      }
      if (mails) setServerMails(mails);
    } finally {
      setLoadingMore(false);
    }
  };

  const onListScroll = (e: React.UIEvent<HTMLDivElement>) => {
    const el = e.currentTarget;
    if (el.scrollHeight - el.scrollTop - el.clientHeight < 300) loadOlder();
  };
  const [refreshing, setRefreshing] = useState(false);
  const [fullBody, setFullBody] = useState<any>(null);
  const [bodyLoading, setBodyLoading] = useState(false);
  const [showMessageDetails, setShowMessageDetails] = useState(false);
  const [showRemote, setShowRemote] = useState(() => localStorage.getItem('emailclient:showRemote') !== '0');

  const toggleRemote = () => setShowRemote((enabled) => {
    localStorage.setItem('emailclient:showRemote', enabled ? '0' : '1');
    return !enabled;
  });

  // Sanitized HTML; remote images are shown unless switched off in the header.
  const cleanHtml = useMemo(() => {
    const raw = (fullBody as any)?.html || '';
    let out = DOMPurify.sanitize(raw, { ADD_ATTR: ['target'] });
    if (!showRemote) {
      out = out.replace(/<img\b[^>]*\bsrc\s*=\s*(["'])https?:[^"']*\1[^>]*>/gi, '');
    }
    return out;
  }, [fullBody, showRemote]);

  const openLink = (e: React.MouseEvent) => {
    const a = (e.target as HTMLElement).closest?.('a[href]') as HTMLAnchorElement | null;
    if (!a) return;
    e.preventDefault();
    const url = a.getAttribute('href') || '';
    if (!/^https?:/i.test(url)) return;
    if (inWails()) BrowserOpenURL(url);
    else window.open(url, '_blank', 'noopener');
  };
  const [sendStatus, setSendStatus] = useState('');
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [storage, setStorage] = useState<any>(null);

  const openSettings = async () => {
    setSettingsOpen(true);
    if (inWails()) {
      try { setStorage(await StorageInfo()); } catch { /* ignore */ }
    }
  };

  const removeAccount = async (email: string) => {
    if (!inWails()) return;
    try { await RemoveAccount(email); } catch { /* ignore */ }
    const rest = accounts.filter((a) => a.email !== email);
    setAccounts(rest);
    if (activeEmail === email) {
      if (rest.length) {
        setActiveEmail(rest[0].email);
        const mails = await FetchMail(rest[0].email, 'inbox').catch(() => null);
        if (mails) setServerMails(mails);
        loadCounts(rest[0].email);
      } else {
        setActiveEmail(''); setServerMails([]); setFolderCounts({}); setFullBody(null);
      }
    }
    try { setStorage(await StorageInfo()); } catch { /* ignore */ }
  };
  const [online, setOnline] = useState(() => typeof navigator === 'undefined' || navigator.onLine);
  useEffect(() => {
    const up = () => setOnline(true);
    const down = () => setOnline(false);
    window.addEventListener('online', up);
    window.addEventListener('offline', down);
    return () => { window.removeEventListener('online', up); window.removeEventListener('offline', down); };
  }, []);
  const [composeTo, setComposeTo] = useState('');
  const [composeSubject, setComposeSubject] = useState('');
  const [composeBody, setComposeBody] = useState('');
  const [composeFiles, setComposeFiles] = useState<Array<{ filename: string; mime: string; size: number; data: string }>>([]);

  const pickFiles = async (list: FileList | null) => {
    if (!list) return;
    const picked: typeof composeFiles = [];
    let total = composeFiles.reduce((n, f) => n + f.size, 0);
    for (const f of Array.from(list)) {
      if (total + f.size > 20 << 20) { setSendStatus('Files over 20MB total — remove something.'); break; }
      const data = await new Promise<string>((res, rej) => {
        const r = new FileReader();
        r.onload = () => res(String(r.result).split(',')[1] || '');
        r.onerror = rej;
        r.readAsDataURL(f);
      });
      total += f.size;
      picked.push({ filename: f.name, mime: f.type, size: f.size, data });
    }
    setComposeFiles((prev) => [...prev, ...picked]);
  };

  const loadCounts = async (email: string) => {
    if (!inWails() || !email) return;
    try {
      const folders = await ListFolders(email);
      const map: Record<string, number> = {};
      const totals: Record<string, number> = {};
      (folders || []).forEach((f: any) => { map[f.key] = f.unseen; totals[f.key] = f.total; });
      setFolderCounts(map);
      setFolderTotals(totals);
    } catch { /* keep old counts */ }
  };

  const refreshAccounts = async () => {
    if (!inWails()) return;
    try {
      const accs = await ListAccounts();
      setAccounts(accs || []);
      if (accs?.length && !activeEmail) {
        setActiveEmail(accs[0].email);
        const mails = await FetchMail(accs[0].email, activeFolder);
        if (mails?.length) setServerMails(mails);
        loadCounts(accs[0].email);
        StartBackfill(accs[0].email, activeFolder);
      }
    } catch { /* mock fallback */ }
    finally { setAccountsLoaded(true); }
  };

  // top-bar refresh: re-fetch current folder + update sidebar counts
  const doRefresh = async () => {
    if (!inWails() || !activeEmail || refreshing) return;
    setRefreshing(true);
    setExhausted(false);
    try {
      const mails = await FetchMail(activeEmail, activeFolder);
      if (mails) setServerMails(mails);
      const rec = await Reconcile(activeEmail, activeFolder).catch(() => '');
      if (rec && !rec.startsWith('already')) {
        const mails2 = await FetchMail(activeEmail, activeFolder).catch(() => null);
        if (mails2) setServerMails(mails2);
      }
      await loadCounts(activeEmail);
    } finally {
      setRefreshing(false);
    }
  };

  useEffect(() => { refreshAccounts(); }, []);
  // live push: background IDLE in Go emits mail:inbox on arrivals
  useEffect(() => {
    if (!inWails() || !activeEmail) return;
    StartWatch(activeEmail).catch(() => {});
    const off = EventsOn('mail:inbox', () => {
      setExhausted(false);
      const folder = activeFolderRef.current;
      FetchMail(activeEmail, folder).then((m) => {
        if (m && activeFolderRef.current === folder) setServerMails(m);
      }).catch(() => {});
      loadCounts(activeEmail);
    });
    return () => { EventsOff('mail:inbox'); off(); StopWatch(activeEmail); };
  }, [activeEmail]);
  useEffect(() => {
    if (!inWails() || !activeEmail) return;
    setExhausted(false);
    setSelectedId('');
    setFullBody(null);
    setServerMails([]);
    const folder = activeFolder;
    FetchMail(activeEmail, folder).then((m) => {
      if (m && activeFolderRef.current === folder) setServerMails(m);
    }).catch(() => {});
    StartBackfill(activeEmail, folder);
  }, [activeFolder]);

  // load full MIME body when selection changes (live accounts only)
  const markedRead = useRef<Set<string>>(new Set());
  useEffect(() => {
    if (!inWails() || !activeEmail || !selectedId) { setFullBody(null); return; }
    setShowMessageDetails(false);
    const uid = Number(selectedId);
    if (!uid) return; // mock ids are non-numeric
    setBodyLoading(true);
    GetMessage(activeEmail, activeFolder, uid)
      .then((f) => {
        setFullBody(f);
        // opening marks read: clear the row dot + decrement the badge now
        const key = activeFolder + ':' + selectedId;
        const row = serverMails.find((m: any) => String(m.uid ?? m.id) === selectedId);
        if (row?.unread && !markedRead.current.has(key)) {
          markedRead.current.add(key);
          setServerMails((prev) => prev.map((m: any) => (String(m.uid ?? m.id) === selectedId ? { ...m, unread: false } : m)));
          setFolderCounts((c) => ({ ...c, [activeFolder]: Math.max(0, (c[activeFolder] ?? 1) - 1) }));
        }
      })
      .catch(() => setFullBody(null))
      .finally(() => setBodyLoading(false));
  }, [selectedId, activeEmail, activeFolder]);

  const live = inWails() && !!activeEmail;
  const liveBody = live && (bodyLoading || fullBody);
  const hasMore = live && !exhausted && (folderTotals[activeFolder] ?? 0) > serverMails.length;
  const source: any[] = live
    ? serverMails.map((m: any, i: number) => ({
        id: String(m.uid ?? m.id ?? i),
        from: m.from || '(no sender)',
        subject: m.subject || '(no subject)',
        preview: m.preview || m.subject || '',
        date: m.date || '',
        avatar: m.avatar || (m.from || '?').slice(0, 1).toUpperCase(),
        unread: !!m.unread,
        folder: m.folder || activeFolder,
        to: m.to || activeEmail,
      }))
    : mockEmails;

  const filtered = useMemo(() => {
    const { ops, text } = parseSearch(query);
    const t = text.toLowerCase();
    return source.filter((e: any) => {
      if (e.folder !== activeFolder) return false;
      if (ops.from && !(e.from || '').toLowerCase().includes(ops.from.toLowerCase())) return false;
      if (ops.subject && !(e.subject || '').toLowerCase().includes(ops.subject.toLowerCase())) return false;
      if (ops.to && !((e.to || '').toLowerCase().includes(ops.to.toLowerCase()))) return false;
      if (t && !((e.from + e.subject + e.preview).toLowerCase().includes(t))) return false;
      return true;
    });
  }, [activeFolder, query, live, serverMails]);

  const selected = source.find((e: any) => e.id === selectedId) ?? filtered[0];
  const searchText = parseSearch(query).text;

  const connectAccount = async () => {
    if (!accEmail || !accPass) { setAccStatus('Enter email + password'); return; }
    if (!inWails()) {
      setAccStatus(`Looking up settings for ${accEmail.split('@')[1] || ''}... (preview mode — run via wails dev for live IMAP)`);
      return;
    }
    try {
      const cfg: any = await DetectSettings(accEmail);
      setAccStatus(`Found ${cfg.imapHost}:${cfg.imapPort} (${cfg.source}) — logging in...`);
      const res = await AddAccount(accName, accEmail, accPass);
      setAccStatus(res);
      await refreshAccounts();
      if (res.startsWith('Connected')) {
        setActiveEmail(accEmail.toLowerCase());
        const mail = accEmail.toLowerCase();
        try {
          const counts = await RefreshAll(mail);
          if (counts) setFolderCounts(counts as any);
        } catch { /* fall through to single fetch */ }
        const mails = await FetchMail(mail, 'inbox');
        if (mails?.length) setServerMails(mails);
        StartBackfill(mail, 'inbox');
      }
    } catch (e: any) {
      setAccStatus('Failed: ' + (e?.message || e));
    }
  };

  const stripHtml = (h: string) => (h || '').replace(/<[^>]*>/g, ' ').replace(/\s+/g, ' ').trim();

  // search: debounced + from:/subject:/to: operators
  const [debouncedQuery, setDebouncedQuery] = useState('');
  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(query), 300);
    return () => clearTimeout(t);
  }, [query]);

  function parseSearch(q: string) {
    const ops: Record<string, string> = {};
    const text = q.replace(/(from|subject|to):("[^"]+"|\S+)/gi, (_, k, v) => {
      ops[k.toLowerCase()] = v.replace(/^"|"$/g, '');
      return '';
    }).trim();
    return { ops, text };
  }

  useEffect(() => {
    if (!inWails() || !activeEmail) return;
    if (!debouncedQuery) {
      FetchMail(activeEmail, activeFolder).then((m) => { if (m) setServerMails(m); }).catch(() => {});
      return;
    }
    const { text } = parseSearch(debouncedQuery);
    ListCached(activeEmail, activeFolder, text).then((m) => { if (m) setServerMails(m); }).catch(() => {});
  }, [debouncedQuery]);

  const highlight = (s: string, needle: string) => {
    if (!needle) return s;
    const i = s.toLowerCase().indexOf(needle.toLowerCase());
    if (i < 0) return s;
    return (<>{s.slice(0, i)}<mark className="bg-yellow-200 rounded-sm">{s.slice(i, i + needle.length)}</mark>{s.slice(i + needle.length)}</>);
  };

  const openCompose = (mode: 'new' | 'reply' | 'forward') => {
    setComposeFiles([]);
    setSendStatus('');
    if (mode === 'new') {
      try {
        const d = JSON.parse(localStorage.getItem('emailclient:draft') || 'null');
        if ((d?.to || d?.subject || d?.body) && !composeTo && !composeSubject && !composeBody) {
          setComposeTo(d.to || ''); setComposeSubject(d.subject || ''); setComposeBody(d.body || '');
        } else if (!composeTo && !composeSubject && !composeBody) {
          setComposeTo(''); setComposeSubject(''); setComposeBody('');
        }
      } catch { setComposeTo(''); setComposeSubject(''); setComposeBody(''); }
      setComposeOpen(true);
      return;
    }
    const fromMail = (live && (fullBody as any)?.from) || (selected as any)?.fromEmail || (selected as any)?.from || '';
    const subj = selected?.subject || '';
    const prefix = mode === 'reply' ? 'Re: ' : 'Fwd: ';
    const cleanSubj = /^(re|fwd?):\s/i.test(subj) ? subj : prefix + subj;
    const origText = (live && ((fullBody as any)?.text || stripHtml((fullBody as any)?.html || ''))) || (selected as any)?.preview || '';
    const quote = `\n\n--- On ${selected?.date || ''}, ${fromMail} wrote: ---\n${origText}`;
    setComposeTo(mode === 'reply' ? fromMail : '');
    setComposeSubject(cleanSubj);
    setComposeBody(quote);
    setComposeOpen(true);
  };

  // draft autosave while composing
  useEffect(() => {
    if (!composeOpen) return;
    const t = setTimeout(() => {
      localStorage.setItem('emailclient:draft', JSON.stringify({ to: composeTo, subject: composeSubject, body: composeBody }));
    }, 800);
    return () => clearTimeout(t);
  }, [composeTo, composeSubject, composeBody, composeOpen]);

  const clearDraft = () => localStorage.removeItem('emailclient:draft');

  const escapeHtml = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  const sendCompose = async () => {
    if (!inWails() || !activeEmail) { clearDraft(); setComposeOpen(false); return; }
    setSendStatus('Sending...');
    try {
      const htmlBody = escapeHtml(composeBody).replace(/\n/g, '<br>');
      const res = await SendMail(activeEmail, composeTo, composeSubject, htmlBody, composeFiles.map(({ filename, mime, data }) => ({ filename, mime, data })));
      setSendStatus(res);
      if (res === 'Sent.') { setComposeOpen(false); setComposeFiles([]); clearDraft(); }
    } catch (e: any) {
      setSendStatus('Send failed: ' + (e?.message || e));
    }
  };

  // premium first-run: desktop app with no accounts → landing + sign-in
  if (inWails() && accountsLoaded && accounts.length === 0) {
    return (
      <div className="h-screen flex text-[#1f1b2d]">
        <div className="w-[46%] text-white flex flex-col justify-center px-14" style={{ background: 'linear-gradient(150deg,#7c3aed 0%,#4c1d95 70%)' }}>
          <div className="flex items-center gap-2 font-semibold text-[20px]">
            <div className="w-8 h-8 rounded-lg bg-white/20 flex items-center justify-center"><Send size={16} /></div>
            <span>space<span className="font-bold">mail</span></span>
          </div>
          <h1 className="mt-10 text-[38px] font-bold leading-tight">Mail,<br />minus the noise.</h1>
          <p className="mt-3 text-white/75 text-[15px]">One calm inbox for every account you own.</p>
          <ul className="mt-8 space-y-4 text-[14px]">
            <li className="flex gap-3"><ShieldCheck size={18} className="shrink-0 mt-0.5" /> No server settings — we detect Zoho, Gmail, cPanel automatically</li>
            <li className="flex gap-3"><Download size={18} className="shrink-0 mt-0.5" /> Offline-first — your mail stays on your computer</li>
            <li className="flex gap-3"><Search size={18} className="shrink-0 mt-0.5" /> Instant search across everything</li>
          </ul>
        </div>
        <div className="flex-1 bg-[#f7f6f8] flex items-center justify-center p-8 overflow-y-auto">
          <div className="w-[380px] bg-white rounded-2xl shadow-xl p-7">
            <h2 className="font-bold text-[20px]">Sign in to your email</h2>
            <p className="text-[13px] text-neutral-500 mt-1">Your password stays in your computer's keychain.</p>
            <div className="mt-5 space-y-2.5 text-sm">
              <input value={accName} onChange={(e) => setAccName(e.target.value)} placeholder="Your name" className="w-full border border-black/15 rounded-lg px-3 py-2.5 outline-none focus:border-violet-500" />
              <input value={accEmail} onChange={(e) => setAccEmail(e.target.value)} placeholder="you@yourdomain.com" className="w-full border border-black/15 rounded-lg px-3 py-2.5 outline-none focus:border-violet-500" />
              <input value={accPass} onChange={(e) => setAccPass(e.target.value)} type="password" placeholder="Password / app password" className="w-full border border-black/15 rounded-lg px-3 py-2.5 outline-none focus:border-violet-500" />
            </div>
            {accStatus && <div className="mt-3 text-[13px] bg-violet-50 text-violet-700 rounded-lg px-3 py-2">{accStatus}</div>}
            <button onClick={connectAccount} className="mt-3 w-full text-white text-sm font-semibold rounded-full py-2.5 hover:brightness-110" style={{ background: PURPLE }}>Connect my email</button>
            <div className="flex items-center gap-3 my-4 text-[12px] text-neutral-400">
              <span className="flex-1 h-px bg-black/10" /> or continue with <span className="flex-1 h-px bg-black/10" />
            </div>
            <div className="flex gap-2">
              <button onClick={() => oauthSignIn('google')} className="flex-1 px-3 py-2.5 rounded-full border border-black/15 text-[13px] font-semibold hover:bg-neutral-50">Google</button>
              <button onClick={() => oauthSignIn('microsoft')} className="flex-1 px-3 py-2.5 rounded-full border border-black/15 text-[13px] font-semibold hover:bg-neutral-50">Microsoft</button>
            </div>
            {(!oauthReady.google || !oauthReady.microsoft) && (
              <button onClick={() => setShowOauthSetup((v) => !v)} className="mt-3 text-[12px] text-neutral-500 underline">
                {showOauthSetup ? 'Hide developer setup' : 'Developer setup'}
              </button>
            )}
            {showOauthSetup && (
              <input value={oauthID} onChange={(e) => setOauthID(e.target.value)} placeholder="OAuth client ID override (dev builds only)" className="mt-2 w-full border border-black/15 rounded-lg px-3 py-2 outline-none focus:border-violet-500 text-sm" />
            )}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="h-screen flex flex-col bg-[#f7f6f8] text-[#1f1b2d]">
      {!online && (
        <div className="bg-amber-100 text-amber-900 text-[13px] text-center py-1.5 px-4 shrink-0">
          You're offline — showing saved mail. New messages will arrive when you reconnect.
        </div>
      )}
      {/* TOP BAR */}
      <header className="h-[60px] bg-white border-b border-black/10 flex items-center gap-4 px-4 shrink-0">
        <Logo />
        <div className="flex-1 flex justify-center">
          <div className="w-[420px] max-w-full flex items-center gap-2 bg-white border border-black/15 rounded-full px-4 py-1.5 shadow-sm">
            <Search size={15} className="text-neutral-500" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search mail"
              className="outline-none text-sm w-full bg-transparent placeholder:text-neutral-500"
            />
          </div>
        </div>
        <button className="text-[13px] font-medium px-3 py-1.5 rounded-full bg-violet-100 text-violet-700">
          Enjoying Spacemail?
        </button>
        <div className="flex items-center gap-1 text-neutral-600">
          <button className="p-2 hover:bg-neutral-100 rounded-full"><HelpCircle size={17} /></button>
          <span className="w-px h-5 bg-black/10 mx-1" />
          <button onClick={doRefresh} title="Refresh" className="p-2 hover:bg-neutral-100 rounded-full"><RefreshCw size={16} className={refreshing ? 'animate-spin' : ''} /></button>
          <button
            type="button"
            role="switch"
            aria-checked={showRemote}
            onClick={toggleRemote}
            title={showRemote ? 'Remote images: on' : 'Remote images: off'}
            className="flex items-center gap-1.5 rounded-full px-2 py-1.5 hover:bg-neutral-100"
          >
            <span className="hidden lg:inline text-[11px] font-medium">Images</span>
            <span className={`relative h-4 w-7 rounded-full transition-colors ${showRemote ? 'bg-violet-600' : 'bg-neutral-300'}`}>
              <span className={`absolute top-0.5 h-3 w-3 rounded-full bg-white shadow-sm transition-transform ${showRemote ? 'translate-x-3.5' : 'translate-x-0.5'}`} />
            </span>
          </button>
          <button onClick={openSettings} title="Settings" className="p-2 hover:bg-neutral-100 rounded-full"><Settings size={17} /></button>
          <button
            onClick={() => setAccountOpen(true)}
            className="w-8 h-8 rounded-full bg-violet-100 text-violet-700 font-semibold ml-1"
            title="Add account"
          >I</button>
        </div>
      </header>

      <div className="flex-1 flex min-h-0">
        {/* SIDEBAR */}
        <aside className="w-[220px] bg-white border-r border-black/10 flex flex-col shrink-0">
          <div className="p-3">
            <button
              onClick={() => openCompose('new')}
              className="w-full text-white text-sm font-semibold rounded-full py-2.5 hover:brightness-110"
              style={{ background: PURPLE }}
            >Compose</button>
          </div>
          <nav className="text-[13.5px]">
            {[
              { id: 'inbox', label: 'Inbox', icon: <Inbox size={15} />, count: live ? (folderCounts['inbox'] ?? 0) : 2 },
              { id: 'drafts', label: 'Drafts', icon: <FileText size={15} />, count: live ? (folderCounts['drafts'] ?? 0) : 0 },
              { id: 'sent', label: 'Sent', icon: <Send size={15} />, count: live ? (folderCounts['sent'] ?? 0) : 0 },
              { id: 'archive', label: 'Archive', icon: <Archive size={15} />, count: live ? (folderCounts['archive'] ?? 0) : 0 },
              { id: 'spam', label: 'Spam', icon: <Ban size={15} />, count: live ? (folderCounts['spam'] ?? 0) : 0 },
              { id: 'trash', label: 'Trash', icon: <Trash2 size={15} />, count: live ? (folderCounts['trash'] ?? 0) : 0 },
            ].map((f) => (
              <button
                key={f.id}
                onClick={() => setActiveFolder(f.id)}
                className={`w-full flex items-center gap-2.5 px-4 py-[7px] ${activeFolder === f.id ? 'bg-neutral-100 font-semibold' : 'text-neutral-700 hover:bg-neutral-50'}`}
              >
                <span className={activeFolder === f.id ? 'text-violet-700' : 'text-neutral-500'}>{f.icon}</span>
                {f.label}
                {f.count > 0 && <span className="ml-auto text-[12px] font-bold text-violet-600">{f.count}</span>}
              </button>
            ))}
          </nav>
          <div className="px-4 pt-6 space-y-3 text-[13px] text-neutral-700 border-t border-black/10 mt-4">
            <button className="flex items-center gap-1 hover:text-black">Spacemail updates <ExternalLink size={13} className="ml-auto" /></button>
            <button className="flex items-center gap-1 hover:text-black">Spacemail roadmap <ExternalLink size={13} className="ml-auto" /></button>
          </div>
          <div className="mt-auto p-4 border-t border-black/10">
            <div className="flex items-center justify-between text-[13px] font-semibold">
              <span>Storage</span>
              <button className="flex items-center gap-1 text-[12px] bg-neutral-100 rounded-full px-2.5 py-1">Add Storage <ExternalLink size={12} /></button>
            </div>
            <div className="h-1 bg-neutral-200 rounded-full mt-3" />
            <div className="flex justify-between text-[11px] text-neutral-500 mt-1"><span><b className="text-black">0 MB</b> of 5 GB</span><span>0%</span></div>
          </div>
        </aside>

        {/* LIST */}
        <section className="w-[320px] bg-white border-r border-black/10 flex flex-col shrink-0">
          <div className="flex items-center gap-2 px-3 py-2 border-b border-black/5">
            {live && folderTotals[activeFolder] != null && (
              <span className="text-[11.5px] text-neutral-500">{filtered.length} shown · {folderTotals[activeFolder]} total</span>
            )}
            <button className="ml-auto p-2 rounded-full bg-neutral-100"><SlidersHorizontal size={15} /></button>
          </div>
          <div className="flex-1 overflow-y-auto" onScroll={onListScroll}>
            {filtered.map((m) => (
              <div
                key={m.id}
                onClick={() => setSelectedId(m.id)}
                className={`flex gap-2.5 px-3 py-3 cursor-pointer border-b border-black/5 ${selectedId === m.id ? 'bg-violet-100/70' : 'hover:bg-neutral-50'}`}
              >
                <input type="checkbox" checked={selectedId === m.id} readOnly className="mt-4 accent-violet-600" />
                <div className="w-9 h-9 rounded-full bg-violet-100 text-violet-700 flex items-center justify-center font-semibold shrink-0 mt-1">{m.avatar}</div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5 text-[13px]">
                    {m.unread && <span className="w-1.5 h-1.5 rounded-full bg-violet-600" />}
                    <span className="font-semibold truncate">{highlight(m.from, searchText)}</span>
                    <span className="ml-auto text-[12px] text-neutral-500 shrink-0">{m.date}</span>
                  </div>
                  <div className="text-[13px] truncate">{highlight(m.subject, searchText)}</div>
                  <div className="text-[12.5px] text-neutral-500 truncate">{m.preview}</div>
                </div>
              </div>
            ))}
            {filtered.length === 0 && <div className="p-8 text-center text-sm text-neutral-500">No messages</div>}
            {loadingMore && <div className="p-3 text-center text-[12px] text-neutral-500">Loading older mail…</div>}
            {exhausted && filtered.length > 0 && <div className="p-3 text-center text-[12px] text-neutral-400">You're all caught up.</div>}
          </div>
        </section>

        {/* READER */}
        <main className="flex-1 min-w-0 bg-[#fafafa] flex flex-col">
          <div className="flex items-center gap-1 px-5 py-2.5 bg-white border-b border-black/10 text-neutral-600">
            <button title="Reply" onClick={() => openCompose('reply')} className="p-2 hover:bg-black/5 rounded-md"><Reply size={16} /></button>
            <button title="Reply all" onClick={() => openCompose('reply')} className="p-2 hover:bg-black/5 rounded-md"><ReplyAll size={16} /></button>
            <button title="Forward" onClick={() => openCompose('forward')} className="p-2 hover:bg-black/5 rounded-md"><Forward size={16} /></button>
            <button title="Print" onClick={() => window.print()} className="p-2 hover:bg-black/5 rounded-md"><Printer size={16} /></button>
            {[Archive, Star, FileText, Trash2, Ban, Download, Code2].map((Icon, i) => (
              <button key={i} className="p-2 hover:bg-black/5 rounded-md"><Icon size={16} /></button>
            ))}
          </div>
          <div className="flex-1 overflow-y-auto px-5 py-5">
            <h1 className="mx-auto mb-4 w-full max-w-[980px] text-[22px] font-bold tracking-tight">{selected?.subject || (live ? 'Mailbox' : '')}</h1>
            <div className="mx-auto w-full max-w-[980px] bg-white rounded-[20px] border border-black/[0.03] shadow-sm overflow-hidden">
              {live && selected ? (
              <>
              <div className="flex items-start gap-3 px-6 py-5">
                <div className="w-10 h-10 rounded-full bg-violet-100 text-violet-700 flex items-center justify-center font-semibold text-lg">{(selected.from || '?').slice(0, 1).toUpperCase()}</div>
                <div className="min-w-0">
                  <div className="truncate text-[14px]"><b>{(fullBody as any)?.from || selected.from}</b></div>
                  <button
                    type="button"
                    onClick={() => setShowMessageDetails((open) => !open)}
                    className="flex max-w-full items-center gap-1 truncate text-[13px] text-neutral-500 hover:text-neutral-800"
                    aria-expanded={showMessageDetails}
                  >
                    <span className="truncate">To: {(fullBody as any)?.to || selected.to}</span>
                    <ChevronDown size={14} className={`shrink-0 transition-transform ${showMessageDetails ? 'rotate-180' : ''}`} />
                  </button>
                </div>
                <div className="ml-auto shrink-0 pt-1 text-[12px] text-neutral-500">{selected.date}</div>
              </div>
              {showMessageDetails && (
                <dl className="grid gap-x-4 gap-y-2 border-t border-black/5 px-6 py-4 text-[13px] sm:grid-cols-[72px_minmax(0,1fr)]">
                  {[
                    ['From', (fullBody as any)?.from || selected.from],
                    ['To', (fullBody as any)?.to || selected.to],
                    ['Cc', (fullBody as any)?.cc],
                    ['Bcc', (fullBody as any)?.bcc],
                    ['Reply-To', (fullBody as any)?.replyTo],
                    ['Subject', (fullBody as any)?.subject || selected.subject],
                    ['Date', (fullBody as any)?.date || selected.date],
                    ['Message-ID', (fullBody as any)?.messageID],
                  ].filter(([, value]) => value).map(([label, value]) => (
                    <div key={label} className="contents">
                      <dt className="font-medium text-neutral-500">{label}</dt>
                      <dd className="min-w-0 break-words text-neutral-800">{value}</dd>
                    </div>
                  ))}
                </dl>
              )}
              </>
              ) : (
              <div className="flex items-center gap-3 px-6 py-4">
                <div className="w-10 h-10 rounded-full bg-violet-100 text-violet-700 flex items-center justify-center font-semibold text-lg">S</div>
                <div>
                  <div className="text-[14px]"><b>Spacemail Team</b> <span className="text-violet-600">&lt;hello@spacemail.com&gt;</span></div>
                  <div className="text-[13px] text-neutral-500">To: &lt;info@serendipitytours.org&gt;</div>
                </div>
                <div className="ml-auto flex items-center gap-2 text-[13px] text-neutral-500">
                  October 7, 2:34 PM <Share size={14} />
                  <span className="flex items-center gap-1 bg-violet-100 text-violet-700 rounded-full px-2 py-0.5 text-[12px] font-semibold"><ShieldCheck size={13} /> 0</span>
                </div>
              </div>
              )}
              {/* Attachments (live accounts) */}
              {live && fullBody?.attachments?.length > 0 && (
                <div className="px-6 pb-4">
                  <div className="flex flex-wrap gap-2">
                    {fullBody.attachments.map((a: any, i: number) => {
                      const url = a.data ? `data:${a.mime || 'application/octet-stream'};base64,${a.data}` : '';
                      const kb = a.size > 1048576 ? `${(a.size / 1048576).toFixed(1)} MB` : `${Math.max(1, Math.round(a.size / 1024))} KB`;
                      const isImg = (a.mime || '').startsWith('image/') && a.data;
                      return (
                        <div key={i} className="flex items-center gap-2 border border-black/10 rounded-xl px-3 py-2 bg-neutral-50 text-[13px]">
                          {isImg
                            ? <img src={url} alt={a.filename} className="w-10 h-10 rounded-lg object-cover" />
                            : <Paperclip size={15} className="text-neutral-500" />}
                          <div className="leading-tight">
                            <div className="font-medium max-w-[180px] truncate">{a.filename}</div>
                            <div className="text-neutral-500 text-[12px]">{kb}{!a.data ? ' — too large to preview' : ''}</div>
                          </div>
                          {url && <a href={url} download={a.filename} className="p-1.5 hover:bg-black/5 rounded-md" title="Download"><Download size={15} /></a>}
                        </div>
                      );
                    })}
                  </div>
                </div>
              )}
              {/* Live body (sanitized) for real accounts; static hero for mock */}
              {liveBody ? (
                <div className="border-t border-black/5 px-6 py-6">
                  {bodyLoading && !fullBody ? (
                    <div className="py-10 text-sm text-neutral-500">Loading message…</div>
                  ) : (fullBody as any)?.html ? (
                    <div
                      className="email-html overflow-x-auto text-[14px] leading-relaxed"
                      onClick={openLink}
                      onErrorCapture={(event) => {
                        const image = event.target as HTMLElement;
                        if (image.tagName === 'IMG') image.style.display = 'none';
                      }}
                      dangerouslySetInnerHTML={{ __html: cleanHtml }}
                    />
                  ) : (
                    <pre className="text-[13px] whitespace-pre-wrap font-sans">{(fullBody as any)?.text || 'No body.'}</pre>
                  )}
                </div>
              ) : !activeEmail ? (
              <div className="mx-6 mb-6 bg-[#e4e2e8] rounded-sm overflow-hidden">
                <div className="bg-white m-0 p-0">
                  <div className="flex items-center gap-2 px-8 py-5 border-b border-black/5">
                    <div className="w-6 h-6 rounded-md flex items-center justify-center text-white" style={{ background: PURPLE }}><Send size={12} /></div>
                    <span className="text-[19px]">space<span className="font-semibold">mail</span></span>
                  </div>
                  <div className="grid grid-cols-2">
                    <div className="p-8 text-white flex flex-col justify-center" style={{ background: 'linear-gradient(135deg,#6d28d9,#4c1d95)' }}>
                      <div className="text-[28px] font-bold leading-tight">Welcome to<br />Spacemail</div>
                      <div className="mt-1 text-white/80 text-[14px]">Here's how to get started</div>
                    </div>
                    <div className="bg-[url('https://images.unsplash.com/photo-1573496359142-b8d87734a5a2?w=600')] bg-cover bg-center min-h-[220px]" />
                  </div>
                  <div className="px-8 py-6 text-[14px] leading-relaxed text-neutral-800">
                    You've successfully created your Spacemail account! Time to <b>add the finishing touches</b> and send your first email.
                    <div className="mt-4 font-bold text-[16px]">1. Enable 2FA for extra security</div>
                    <p className="mt-1">Click <b>Settings</b> and find the <b>Security Center</b> box (or just click below).</p>
                    <button className="mt-3 text-white text-sm font-semibold rounded-full px-5 py-2" style={{ background: PURPLE }}>Open Security Center</button>
                  </div>
                </div>
              </div>
              ) : (
              <div className="mx-6 mb-6 bg-white rounded-sm overflow-hidden">
                <div className="px-8 py-16 text-center text-neutral-500 text-sm">
                  {filtered.length ? 'Select a message to read it.' : 'No messages here yet.'}
                </div>
              </div>
              )}
              {live && selected && !bodyLoading && (
                <div className="flex items-center justify-between gap-3 border-t border-black/5 px-6 py-4">
                  <div className="flex flex-wrap items-center gap-4 text-[13px] font-semibold text-violet-700">
                    <button onClick={() => openCompose('reply')} className="flex items-center gap-1.5 hover:text-violet-900"><Reply size={15} /> Reply</button>
                    <button onClick={() => openCompose('reply')} className="flex items-center gap-1.5 hover:text-violet-900"><ReplyAll size={15} /> Reply all</button>
                    <button onClick={() => openCompose('forward')} className="flex items-center gap-1.5 hover:text-violet-900"><Forward size={15} /> Forward</button>
                  </div>
                  <button className="flex shrink-0 items-center gap-1.5 text-[13px] font-medium text-neutral-700 hover:text-black"><Ban size={15} /> Mark as spam</button>
                </div>
              )}
            </div>
          </div>
        </main>
      </div>

      {/* COMPOSE MODAL */}
      {composeOpen && (
        <div className="fixed inset-0 bg-black/30 flex items-end justify-end p-6 z-50" onClick={() => setComposeOpen(false)}>
          <div className="bg-white w-[520px] rounded-2xl shadow-2xl overflow-hidden" onClick={(e) => e.stopPropagation()}>
            <div className="flex items-center px-4 py-3 border-b border-black/10 text-sm font-semibold">
              New message <button className="ml-auto" onClick={() => setComposeOpen(false)}>✕</button>
            </div>
            <div className="p-4 space-y-2 text-sm">
              <input value={composeTo} onChange={(e) => setComposeTo(e.target.value)} placeholder="To" className="w-full border-b border-black/10 outline-none py-1.5" />
              <input value={composeSubject} onChange={(e) => setComposeSubject(e.target.value)} placeholder="Subject" className="w-full border-b border-black/10 outline-none py-1.5" />
              <textarea value={composeBody} onChange={(e) => setComposeBody(e.target.value)} rows={8} placeholder="Write…" className="w-full outline-none py-2 resize-none" />
              {composeFiles.length > 0 && (
                <div className="flex flex-wrap gap-1.5">
                  {composeFiles.map((f, i) => (
                    <span key={i} className="flex items-center gap-1.5 bg-neutral-100 rounded-full pl-3 pr-1.5 py-1 text-[12px]">
                      <Paperclip size={12} className="text-neutral-500" />
                      <span className="max-w-[140px] truncate">{f.filename}</span>
                      <button onClick={() => setComposeFiles((p) => p.filter((_, j) => j !== i))} className="p-0.5 hover:bg-black/10 rounded-full">✕</button>
                    </span>
                  ))}
                </div>
              )}
              {sendStatus && <div className="text-[13px] text-neutral-500">{sendStatus}</div>}
              <div className="flex items-center gap-2">
                <label className="p-2 hover:bg-neutral-100 rounded-full cursor-pointer text-neutral-500" title="Attach files">
                  <Paperclip size={16} />
                  <input type="file" multiple className="hidden" onChange={(e) => { pickFiles(e.target.files); e.target.value = ''; }} />
                </label>
                <div className="flex justify-end gap-2 ml-auto">
                <button onClick={() => { clearDraft(); setComposeOpen(false); }} className="px-4 py-2 text-sm rounded-full bg-neutral-100">Discard</button>
                <button onClick={sendCompose} className="px-5 py-2 text-sm rounded-full text-white font-semibold" style={{ background: PURPLE }}>Send</button>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ADD ACCOUNT MODAL */}
      {accountOpen && (
        <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50" onClick={() => setAccountOpen(false)}>
          <div className="bg-white w-[420px] rounded-2xl shadow-2xl p-6" onClick={(e) => e.stopPropagation()}>
            <h2 className="font-bold text-[17px] flex items-center gap-2"><Plus size={17} /> Add email account</h2>
            <p className="text-[13px] text-neutral-500 mt-1">Works with Zoho, cPanel, Roundcube hosting, Gmail, Outlook. No SMTP ports needed — we auto-detect.</p>
            <div className="mt-4 space-y-2.5 text-sm">
              <input value={accName} onChange={(e) => setAccName(e.target.value)} placeholder="Your name" className="w-full border border-black/15 rounded-lg px-3 py-2 outline-none focus:border-violet-500" />
              <input value={accEmail} onChange={(e) => setAccEmail(e.target.value)} placeholder="you@yourdomain.com" className="w-full border border-black/15 rounded-lg px-3 py-2 outline-none focus:border-violet-500" />
              <input value={accPass} onChange={(e) => setAccPass(e.target.value)} type="password" placeholder="Password / app password" className="w-full border border-black/15 rounded-lg px-3 py-2 outline-none focus:border-violet-500" />
            </div>
            {accStatus && <div className="mt-3 text-[13px] bg-violet-50 text-violet-700 rounded-lg px-3 py-2">{accStatus}</div>}
            <div className="mt-3 pt-3 border-t border-black/10">
              <div className="text-[12px] text-neutral-500 mb-2">No app password? Sign in with your provider instead:</div>
              <div className="flex gap-2">
                <button onClick={() => oauthSignIn('google')} className="flex-1 px-3 py-2 rounded-full border border-black/15 text-[13px] font-semibold hover:bg-neutral-50">Sign in with Google</button>
                <button onClick={() => oauthSignIn('microsoft')} className="flex-1 px-3 py-2 rounded-full border border-black/15 text-[13px] font-semibold hover:bg-neutral-50">Sign in with Microsoft</button>
              </div>
              {(!oauthReady.google || !oauthReady.microsoft) && (
                <button onClick={() => setShowOauthSetup((v) => !v)} className="mt-2 text-[12px] text-neutral-500 underline">
                  {showOauthSetup ? 'Hide developer setup' : 'Developer setup'}
                </button>
              )}
              {showOauthSetup && (
                <input value={oauthID} onChange={(e) => setOauthID(e.target.value)} placeholder="OAuth client ID override (dev builds only)" className="mt-2 w-full border border-black/15 rounded-lg px-3 py-2 outline-none focus:border-violet-500" />
              )}
            </div>
            <div className="mt-4 flex justify-end gap-2 text-sm">
              <button onClick={() => setAccountOpen(false)} className="px-4 py-2 rounded-full bg-neutral-100">Close</button>
              <button onClick={connectAccount} className="px-5 py-2 rounded-full text-white font-semibold flex items-center gap-1" style={{ background: PURPLE }}>Connect <ChevronDown size={0} /></button>
            </div>
          </div>
        </div>
      )}

      {/* SETTINGS MODAL */}
      {settingsOpen && (
        <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50" onClick={() => setSettingsOpen(false)}>
          <div className="bg-white w-[420px] rounded-2xl shadow-2xl p-6" onClick={(e) => e.stopPropagation()}>
            <h2 className="font-bold text-[17px] flex items-center gap-2"><Settings size={17} /> Settings</h2>
            <div className="mt-4 text-sm font-semibold">Accounts</div>
            <div className="mt-1.5 space-y-1.5">
              {accounts.length === 0 && <div className="text-[13px] text-neutral-500">No accounts yet — click the avatar (top right) to add one.</div>}
              {accounts.map((a) => (
                <div key={a.email} className="flex items-center gap-2 border border-black/10 rounded-lg px-3 py-2 text-[13px]">
                  <span className="truncate font-medium">{a.email}</span>
                  {activeEmail === a.email && <span className="text-[11px] bg-violet-100 text-violet-700 rounded-full px-2 py-0.5">active</span>}
                  <button onClick={() => removeAccount(a.email)} className="ml-auto text-neutral-500 hover:text-red-600 p-1" title="Remove account"><Trash2 size={15} /></button>
                </div>
              ))}
            </div>
            <div className="mt-4 text-sm font-semibold">Storage</div>
            <div className="text-[13px] text-neutral-500 mt-1">
              {storage ? `${storage.messages} messages cached (${(storage.bytes / 1048576).toFixed(1)} MB)` : '…'}
            </div>
            <div className="mt-4 flex justify-end text-sm">
              <button onClick={() => setSettingsOpen(false)} className="px-4 py-2 rounded-full bg-neutral-100">Close</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
