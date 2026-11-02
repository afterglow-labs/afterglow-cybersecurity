export const en = {
  title: "Automatic follow-up waves",
  description: "When a task times out or stops with open work, schedule up to three follow-up tasks across its entire lineage. Each wave inherits authorized context and has a strict time limit.",
  enabled: "Enable follow-up waves",
  delay: "Delay before starting (minutes)",
  budget: "Maximum time per wave (minutes)",
  save: "Save follow-up policy",
  saved: "Follow-up policy saved",
  waves: "Follow-up waves",
  empty: "No follow-up waves for this task.",
  pending: "Scheduled",
  spawning: "Starting",
  spawned: "Started",
  cancelled: "Cancelled",
  wave: "Wave {number}",
  reasonTimeout: "Task timed out",
  reasonGoals: "Open goals remained",
  reasonIntents: "Open work remained",
  child: "Open follow-up task {id}",
} as const;

export const ko: Record<keyof typeof en, string> = {
  title: "자동 후속 작업", description: "작업이 시간 초과되거나 미완료 항목을 남기고 멈추면 전체 계보에서 최대 세 개의 후속 작업을 예약합니다. 각 작업은 허가된 맥락을 상속하며 실행 시간이 제한됩니다.",
  enabled: "후속 작업 사용", delay: "시작 전 대기(분)", budget: "후속 작업당 최대 시간(분)", save: "후속 정책 저장", saved: "후속 정책 저장됨",
  waves: "후속 작업", empty: "이 작업의 후속 작업이 없습니다.", pending: "예약됨", spawning: "시작 중", spawned: "시작됨", cancelled: "취소됨",
  wave: "후속 {number}", reasonTimeout: "작업 시간 초과", reasonGoals: "미완료 목표", reasonIntents: "미완료 작업", child: "후속 작업 {id} 열기",
};
