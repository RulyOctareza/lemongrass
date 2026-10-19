// Copy for the Guide dialog. Each card is one feature the user can find in
// the app. Keep the copy plain, with no em dashes and no "--" anywhere, since
// the app's UI text is held to that rule.

export interface GuideCard {
  id: string
  title: string
  where: string
  purpose: string
  when: string
  example: string
}

export const GUIDE_CARDS: GuideCard[] = [
  {
    id: 'project-switcher',
    title: 'Projects',
    where: 'The project name in the header',
    purpose:
      'Lemongrass works on one project folder at a time. Each project has its own tabs, its own books and its own vault.',
    when: 'When you want to switch to another project, or add a new one.',
    example:
      'Click the project name, pick a project from the list, or choose Add project to point Lemongrass at a new folder.'
  },
  {
    id: 'biblio',
    title: 'Biblio',
    where: 'The sidebar, at the top',
    purpose:
      'Biblio is the project knowledge folder, biblio/. It holds scratchpad tasks, laws, handovers and books, shown as a tree you can open and read.',
    when: 'When you want to see the plans, rules and notes an agent works from.',
    example:
      'Open biblio, then scratchpad, then a task, to read its PRD and activity log.'
  },
  {
    id: 'scratchpad-editor',
    title: 'Scratchpad editor',
    where: 'Biblio, then a file under scratchpad',
    purpose:
      'Scratchpad files open in an editor that saves as you type. Other parts of biblio open read only.',
    when: 'When you want to write or change a plan, a note or a task file yourself.',
    example:
      'Open a PRD under scratchpad, change a line, and the change is saved without a save button.'
  },
  {
    id: 'connector',
    title: 'Connector',
    where: 'Vault, in the sidebar under Biblio',
    purpose:
      'The connector gives agents access to databases and HTTP APIs without showing them the password or token. You create a connection once, then grant an agent a channel that can only do what you allow.',
    when: 'When an agent needs to read or change data in a database or call an API that needs a login.',
    example:
      'Add a MySQL connection, then create a read-only channel on two of its tables for an agent to use for a day.'
  },
  {
    id: 'safety',
    title: 'Safety',
    where: 'Settings, then Safety',
    purpose:
      'Claude tabs run without permission prompts. Safety holds the rules that still block dangerous commands, such as deleting files, changing git history or running database clients. You can add your own rules, globally or for one project.',
    when: 'When you want to change which commands are blocked, or to see what is already blocked.',
    example:
      'Add a rule that blocks a deploy command in one project, and it stays blocked there only.'
  },
  {
    id: 'workgroups',
    title: 'Workgroups',
    where: 'The workgroup list in the sidebar, and the thread in a workgroup tab',
    purpose:
      'A leader agent can start a group of thinkers, each on Claude Code or Codex, to work on one task together. They talk in a shared thread, and you approve the group before any of them starts.',
    when: 'When a task is big enough to split across several agents that need to discuss it.',
    example:
      'Ask a Claude tab to start a workgroup for a review. Read its approval request, then approve the thinkers you want.'
  },
  {
    id: 'node',
    title: 'Node',
    where: 'Vault, then Node, in the sidebar',
    purpose:
      'Node shares some of your books, scratchpad tasks, laws and handovers with someone else\'s agent over a read-only link. Each person gets one token and sets it up once, and you change what it can read at any time.',
    when: 'When someone outside your machine needs to read specific project notes through their own agent.',
    example:
      'Add a token for a colleague, add one book and one task to it, and open the link for two hours.'
  },
  {
    id: 'accounts',
    title: 'Claude accounts',
    where: 'Settings, then Claude accounts',
    purpose:
      'If you use more than one Claude login, add each one here. Each account keeps its own settings and history, and a tab runs on the account it started with.',
    when: 'When you want some tabs or workgroup thinkers to run on a different Claude login.',
    example:
      'Add a second account for a work login, then pick it in the New shell dialog for a work tab.'
  },
  {
    id: 'git',
    title: 'Git panel',
    where: 'Biblio, then Git, in the sidebar',
    purpose:
      'The Git panel shows the changes in the current project, lets you read the diff, commit and push, without leaving Lemongrass.',
    when: 'When you want to review what an agent changed before you commit it.',
    example:
      'Open Git, click a changed file to read its diff, then write a message and commit.'
  },
  {
    id: 'settings',
    title: 'Settings',
    where: 'The cog icon in the header',
    purpose:
      'Settings holds the app-wide options: Claude accounts and Safety rules. Use it to change how Lemongrass runs, not to change a project.',
    when: 'When you want to add an account or change a Safety rule.',
    example:
      'Open the cog, choose Safety, and add a rule.'
  }
]
