export type TaskStatus = 'backlog' | 'drafting' | 'in_review' | 'done';
export type TaskKind = 'content' | 'campaign' | 'email';

export interface Variant {
  id: 'A' | 'B' | 'C';
  label: string;
  body: string;
  seo: number;
  voice: number;
  charCount: number;
  recommended?: boolean;
  note?: string;
}

export interface ContentTask {
  id: string;
  kind: 'content';
  title: string;
  status: TaskStatus;
  productSku?: string;
  productName?: string;
  currentDescription?: string;
  variants: Variant[];
  surfacedAt: string;
}

export interface CampaignChild {
  id: string;
  title: string;
  scheduledFor: string;
  status: TaskStatus;
}

export interface CampaignTask {
  id: string;
  kind: 'campaign';
  title: string;
  status: TaskStatus;
  goal: string;
  audience: string;
  startsOn: string;
  endsOn: string;
  budget: string;
  children: CampaignChild[];
  surfacedAt: string;
}

export interface EmailTask {
  id: string;
  kind: 'email';
  title: string;
  status: TaskStatus;
  subject: string;
  preview: string;
  body: string;
  surfacedAt: string;
}

export type Task = ContentTask | CampaignTask | EmailTask;
