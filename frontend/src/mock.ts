export interface Email {
  id: string;
  from: string;
  fromEmail: string;
  to: string;
  subject: string;
  preview: string;
  body: string;
  date: string;
  avatar: string;
  unread: boolean;
  folder: 'inbox' | 'drafts' | 'sent' | 'archive' | 'spam' | 'trash';
}

export const mockEmails: Email[] = [
  {
    id: '1',
    from: 'Mandela Washington Fellow...',
    fromEmail: 'fellowship@mandela.org',
    to: 'you@serendipitytours.org',
    subject: 'Mandela Washington Fellow...',
    preview: 'Dear Aidah Akariza , Congratulat...',
    body: `<p>Dear Aidah Akariza,</p><p>Congratulations! You have been selected...</p>`,
    date: 'Oct 12',
    avatar: 'M',
    unread: true,
    folder: 'inbox',
  },
  {
    id: '2',
    from: 'Mandela Washington Fellow...',
    fromEmail: 'fellowship@mandela.org',
    to: 'you@serendipitytours.org',
    subject: 'Mandela Washington Fellow...',
    preview: 'Dear Applicant, Thank you for y...',
    body: `<p>Dear Applicant,</p><p>Thank you for your application...</p>`,
    date: 'Oct 12',
    avatar: 'M',
    unread: true,
    folder: 'inbox',
  },
  {
    id: '3',
    from: 'Spacemail Team',
    fromEmail: 'hello@spacemail.com',
    to: '<info@serendipitytours.org>',
    subject: 'Get started with Spacemail',
    preview: 'Set your password, personalize ...',
    body: 'welcome',
    date: 'Oct 07',
    avatar: 'S',
    unread: false,
    folder: 'inbox',
  },
];

export const folders = [
  { id: 'inbox', label: 'Inbox', count: 2 },
  { id: 'drafts', label: 'Drafts', count: 0 },
  { id: 'sent', label: 'Sent', count: 0 },
  { id: 'archive', label: 'Archive', count: 0 },
  { id: 'spam', label: 'Spam', count: 0 },
  { id: 'trash', label: 'Trash', count: 0 },
] as const;
