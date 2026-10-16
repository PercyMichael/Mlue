export namespace autoconfig {
	
	export class MailConfig {
	    imapHost: string;
	    imapPort: number;
	    imapSSL: boolean;
	    smtpHost: string;
	    smtpPort: number;
	    smtpSSL: boolean;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new MailConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.imapHost = source["imapHost"];
	        this.imapPort = source["imapPort"];
	        this.imapSSL = source["imapSSL"];
	        this.smtpHost = source["smtpHost"];
	        this.smtpPort = source["smtpPort"];
	        this.smtpSSL = source["smtpSSL"];
	        this.source = source["source"];
	    }
	}

}

export namespace mail {
	
	export class Attachment {
	    filename: string;
	    mime: string;
	    size: number;
	    data: string;
	
	    static createFrom(source: any = {}) {
	        return new Attachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filename = source["filename"];
	        this.mime = source["mime"];
	        this.size = source["size"];
	        this.data = source["data"];
	    }
	}
	export class Folder {
	    key: string;
	    mailbox: string;
	    unseen: number;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new Folder(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.mailbox = source["mailbox"];
	        this.unseen = source["unseen"];
	        this.total = source["total"];
	    }
	}
	export class Full {
	    uid: number;
	    subject: string;
	    from: string;
	    to: string;
	    cc: string;
	    bcc: string;
	    replyTo: string;
	    date: string;
	    messageID: string;
	    html: string;
	    text: string;
	    attachments: Attachment[];
	
	    static createFrom(source: any = {}) {
	        return new Full(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uid = source["uid"];
	        this.subject = source["subject"];
	        this.from = source["from"];
	        this.to = source["to"];
	        this.cc = source["cc"];
	        this.bcc = source["bcc"];
	        this.replyTo = source["replyTo"];
	        this.date = source["date"];
	        this.messageID = source["messageID"];
	        this.html = source["html"];
	        this.text = source["text"];
	        this.attachments = this.convertValues(source["attachments"], Attachment);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SendFile {
	    filename: string;
	    mime: string;
	    data: string;
	
	    static createFrom(source: any = {}) {
	        return new SendFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filename = source["filename"];
	        this.mime = source["mime"];
	        this.data = source["data"];
	    }
	}

}

export namespace store {
	
	export class Account {
	    name: string;
	    email: string;
	    imapHost: string;
	    imapPort: number;
	    imapSSL: boolean;
	    smtpHost: string;
	    smtpPort: number;
	    smtpSSL: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.email = source["email"];
	        this.imapHost = source["imapHost"];
	        this.imapPort = source["imapPort"];
	        this.imapSSL = source["imapSSL"];
	        this.smtpHost = source["smtpHost"];
	        this.smtpPort = source["smtpPort"];
	        this.smtpSSL = source["smtpSSL"];
	    }
	}
	export class Message {
	    id: number;
	    account: string;
	    folder: string;
	    uid: number;
	    from: string;
	    fromEmail: string;
	    to: string;
	    subject: string;
	    preview: string;
	    body: string;
	    bodyHTML: boolean;
	    date: string;
	    avatar: string;
	    unread: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.account = source["account"];
	        this.folder = source["folder"];
	        this.uid = source["uid"];
	        this.from = source["from"];
	        this.fromEmail = source["fromEmail"];
	        this.to = source["to"];
	        this.subject = source["subject"];
	        this.preview = source["preview"];
	        this.body = source["body"];
	        this.bodyHTML = source["bodyHTML"];
	        this.date = source["date"];
	        this.avatar = source["avatar"];
	        this.unread = source["unread"];
	    }
	}

}

