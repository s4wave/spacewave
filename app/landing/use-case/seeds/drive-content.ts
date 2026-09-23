// DRIVE_DEMO_FOLDERS are the folders the Drive demo adds beside the
// Quickstart's getting-started guide.
export const DRIVE_DEMO_FOLDERS = ['Photos', 'Projects']

// DriveDemoFile is one file written into the Drive demo.
export interface DriveDemoFile {
  path: string
  content: string
}

const harbor = `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="800" viewBox="0 0 1200 800">
<rect width="1200" height="800" fill="#f2d7c2"/><circle cx="880" cy="230" r="90" fill="#f7eee4"/>
<path d="M0 470 260 300 470 430 700 250 960 420 1200 330V800H0Z" fill="#6f7d8c"/>
<path d="M0 520Q300 470 600 530T1200 520V800H0Z" fill="#2f5063"/>
<path d="M360 560h240l-30 44H390Z" fill="#e9e2d8"/><path d="M470 440v120h8V440Z" fill="#e9e2d8"/>
<path d="M478 450 560 540H478Z" fill="#f7eee4"/>
</svg>
`

// DRIVE_DEMO_FILES are the files the Drive demo writes. The static poster
// lists the same entries.
export const DRIVE_DEMO_FILES: DriveDemoFile[] = [
  { path: 'Photos/harbor.svg', content: harbor },
  {
    path: 'Projects/launch-plan.md',
    content: `# Launch plan

- [x] Draft the announcement
- [ ] Record the demo video
- [ ] Share the Space with the team

Everything here lives in the Space. Open it on another device and it is
already there.
`,
  },
  {
    path: 'Projects/budget.csv',
    content: 'item,cost\nDomain,12\nHosting,0\nCoffee,48\n',
  },
]
