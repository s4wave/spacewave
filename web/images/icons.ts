import type { ReactNode } from 'react'
import type { IconBaseProps } from 'react-icons'
import type { StyleXStyles } from '@stylexjs/stylex'
import { createStyledIcon } from './styled-icon.js'
export { IconContext } from 'react-icons'
export { IconStylesProvider } from './IconStylesProvider.js'
/** IconType renders a shared SVG with caller-owned native styles. */
export type IconType = (
  props: Omit<IconBaseProps, 'className'> & {
    sx?: StyleXStyles
    className?: never
  },
) => ReactNode

import * as iconsfc from 'react-icons/fc'
/** FcGoogle renders its SVG with native styles. */
export const FcGoogle: IconType = /*#__PURE__*/ createStyledIcon(
  iconsfc.FcGoogle,
)

import * as iconslu from 'react-icons/lu'
/** LuActivity renders its SVG with native styles. */
export const LuActivity: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuActivity,
)
/** LuAppWindow renders its SVG with native styles. */
export const LuAppWindow: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuAppWindow,
)
/** LuArchive renders its SVG with native styles. */
export const LuArchive: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuArchive,
)
/** LuArrowDown renders its SVG with native styles. */
export const LuArrowDown: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuArrowDown,
)
/** LuArrowDownAZ renders its SVG with native styles. */
export const LuArrowDownAZ: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuArrowDownAZ,
)
/** LuArrowDownRight renders its SVG with native styles. */
export const LuArrowDownRight: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuArrowDownRight,
)
/** LuArrowLeft renders its SVG with native styles. */
export const LuArrowLeft: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuArrowLeft,
)
/** LuArrowRight renders its SVG with native styles. */
export const LuArrowRight: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuArrowRight,
)
/** LuArrowRightLeft renders its SVG with native styles. */
export const LuArrowRightLeft: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuArrowRightLeft,
)
/** LuArrowUp renders its SVG with native styles. */
export const LuArrowUp: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuArrowUp,
)
/** LuArrowUpAZ renders its SVG with native styles. */
export const LuArrowUpAZ: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuArrowUpAZ,
)
/** LuArrowUpRight renders its SVG with native styles. */
export const LuArrowUpRight: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuArrowUpRight,
)
/** LuBanknote renders its SVG with native styles. */
export const LuBanknote: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuBanknote,
)
/** LuBold renders its SVG with native styles. */
export const LuBold: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuBold)
/** LuBookOpen renders its SVG with native styles. */
export const LuBookOpen: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuBookOpen,
)
/** LuBot renders its SVG with native styles. */
export const LuBot: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuBot)
/** LuBox renders its SVG with native styles. */
export const LuBox: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuBox)
/** LuBoxes renders its SVG with native styles. */
export const LuBoxes: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuBoxes)
/** LuBrainCircuit renders its SVG with native styles. */
export const LuBrainCircuit: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuBrainCircuit,
)
/** LuBriefcase renders its SVG with native styles. */
export const LuBriefcase: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuBriefcase,
)
/** LuBug renders its SVG with native styles. */
export const LuBug: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuBug)
/** LuBuilding2 renders its SVG with native styles. */
export const LuBuilding2: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuBuilding2,
)
/** LuCalendar renders its SVG with native styles. */
export const LuCalendar: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCalendar,
)
/** LuCalendarX renders its SVG with native styles. */
export const LuCalendarX: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCalendarX,
)
/** LuCamera renders its SVG with native styles. */
export const LuCamera: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCamera,
)
/** LuChartLine renders its SVG with native styles. */
export const LuChartLine: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuChartLine,
)
/** LuChartNoAxesColumnIncreasing renders its SVG with native styles. */
export const LuChartNoAxesColumnIncreasing: IconType =
  /*#__PURE__*/ createStyledIcon(iconslu.LuChartNoAxesColumnIncreasing)
/** LuCheck renders its SVG with native styles. */
export const LuCheck: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuCheck)
/** LuChevronDown renders its SVG with native styles. */
export const LuChevronDown: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuChevronDown,
)
/** LuChevronLeft renders its SVG with native styles. */
export const LuChevronLeft: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuChevronLeft,
)
/** LuChevronRight renders its SVG with native styles. */
export const LuChevronRight: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuChevronRight,
)
/** LuChevronUp renders its SVG with native styles. */
export const LuChevronUp: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuChevronUp,
)
/** LuCircle renders its SVG with native styles. */
export const LuCircle: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCircle,
)
/** LuCircleAlert renders its SVG with native styles. */
export const LuCircleAlert: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCircleAlert,
)
/** LuCircleCheck renders its SVG with native styles. */
export const LuCircleCheck: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCircleCheck,
)
/** LuCircleDot renders its SVG with native styles. */
export const LuCircleDot: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCircleDot,
)
/** LuCircleOff renders its SVG with native styles. */
export const LuCircleOff: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCircleOff,
)
/** LuCircleX renders its SVG with native styles. */
export const LuCircleX: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCircleX,
)
/** LuCircuitBoard renders its SVG with native styles. */
export const LuCircuitBoard: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCircuitBoard,
)
/** LuClipboard renders its SVG with native styles. */
export const LuClipboard: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuClipboard,
)
/** LuClipboardCheck renders its SVG with native styles. */
export const LuClipboardCheck: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuClipboardCheck,
)
/** LuClipboardList renders its SVG with native styles. */
export const LuClipboardList: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuClipboardList,
)
/** LuClipboardPaste renders its SVG with native styles. */
export const LuClipboardPaste: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuClipboardPaste,
)
/** LuClock3 renders its SVG with native styles. */
export const LuClock3: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuClock3,
)
/** LuCloud renders its SVG with native styles. */
export const LuCloud: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuCloud)
/** LuCloudDownload renders its SVG with native styles. */
export const LuCloudDownload: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCloudDownload,
)
/** LuCloudOff renders its SVG with native styles. */
export const LuCloudOff: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCloudOff,
)
/** LuCode renders its SVG with native styles. */
export const LuCode: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuCode)
/** LuCommand renders its SVG with native styles. */
export const LuCommand: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCommand,
)
/** LuCompass renders its SVG with native styles. */
export const LuCompass: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCompass,
)
/** LuCopy renders its SVG with native styles. */
export const LuCopy: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuCopy)
/** LuCornerDownLeft renders its SVG with native styles. */
export const LuCornerDownLeft: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCornerDownLeft,
)
/** LuCornerLeftUp renders its SVG with native styles. */
export const LuCornerLeftUp: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCornerLeftUp,
)
/** LuCpu renders its SVG with native styles. */
export const LuCpu: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuCpu)
/** LuCreditCard renders its SVG with native styles. */
export const LuCreditCard: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuCreditCard,
)
/** LuCrown renders its SVG with native styles. */
export const LuCrown: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuCrown)
/** LuDatabase renders its SVG with native styles. */
export const LuDatabase: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuDatabase,
)
/** LuDownload renders its SVG with native styles. */
export const LuDownload: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuDownload,
)
/** LuEllipsis renders its SVG with native styles. */
export const LuEllipsis: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuEllipsis,
)
/** LuEllipsisVertical renders its SVG with native styles. */
export const LuEllipsisVertical: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuEllipsisVertical,
)
/** LuExternalLink renders its SVG with native styles. */
export const LuExternalLink: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuExternalLink,
)
/** LuEye renders its SVG with native styles. */
export const LuEye: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuEye)
/** LuEyeOff renders its SVG with native styles. */
export const LuEyeOff: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuEyeOff,
)
/** LuFile renders its SVG with native styles. */
export const LuFile: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuFile)
/** LuFileQuestion renders its SVG with native styles. */
export const LuFileQuestion: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuFileQuestion,
)
/** LuFileText renders its SVG with native styles. */
export const LuFileText: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuFileText,
)
/** LuFingerprint renders its SVG with native styles. */
export const LuFingerprint: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuFingerprint,
)
/** LuFolder renders its SVG with native styles. */
export const LuFolder: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuFolder,
)
/** LuFolderInput renders its SVG with native styles. */
export const LuFolderInput: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuFolderInput,
)
/** LuFolderOpen renders its SVG with native styles. */
export const LuFolderOpen: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuFolderOpen,
)
/** LuFolderPlus renders its SVG with native styles. */
export const LuFolderPlus: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuFolderPlus,
)
/** LuFolderSync renders its SVG with native styles. */
export const LuFolderSync: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuFolderSync,
)
/** LuFolderTree renders its SVG with native styles. */
export const LuFolderTree: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuFolderTree,
)
/** LuGauge renders its SVG with native styles. */
export const LuGauge: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuGauge)
/** LuGift renders its SVG with native styles. */
export const LuGift: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuGift)
/** LuGitBranch renders its SVG with native styles. */
export const LuGitBranch: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuGitBranch,
)
/** LuGitFork renders its SVG with native styles. */
export const LuGitFork: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuGitFork,
)
/** LuGitMerge renders its SVG with native styles. */
export const LuGitMerge: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuGitMerge,
)
/** LuGithub renders its SVG with native styles. */
export const LuGithub: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuGithub,
)
/** LuGlobe renders its SVG with native styles. */
export const LuGlobe: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuGlobe)
/** LuHammer renders its SVG with native styles. */
export const LuHammer: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuHammer,
)
/** LuHardDrive renders its SVG with native styles. */
export const LuHardDrive: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuHardDrive,
)
/** LuHardDriveDownload renders its SVG with native styles. */
export const LuHardDriveDownload: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuHardDriveDownload,
)
/** LuHash renders its SVG with native styles. */
export const LuHash: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuHash)
/** LuHeart renders its SVG with native styles. */
export const LuHeart: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuHeart)
/** LuHistory renders its SVG with native styles. */
export const LuHistory: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuHistory,
)
/** LuHouse renders its SVG with native styles. */
export const LuHouse: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuHouse)
/** LuIdCard renders its SVG with native styles. */
export const LuIdCard: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuIdCard,
)
/** LuImage renders its SVG with native styles. */
export const LuImage: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuImage)
/** LuImagePlus renders its SVG with native styles. */
export const LuImagePlus: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuImagePlus,
)
/** LuInbox renders its SVG with native styles. */
export const LuInbox: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuInbox)
/** LuInfo renders its SVG with native styles. */
export const LuInfo: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuInfo)
/** LuItalic renders its SVG with native styles. */
export const LuItalic: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuItalic,
)
/** LuKey renders its SVG with native styles. */
export const LuKey: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuKey)
/** LuKeyRound renders its SVG with native styles. */
export const LuKeyRound: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuKeyRound,
)
/** LuKeyboard renders its SVG with native styles. */
export const LuKeyboard: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuKeyboard,
)
/** LuLaptop renders its SVG with native styles. */
export const LuLaptop: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuLaptop,
)
/** LuLayers renders its SVG with native styles. */
export const LuLayers: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuLayers,
)
/** LuLayoutDashboard renders its SVG with native styles. */
export const LuLayoutDashboard: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuLayoutDashboard,
)
/** LuLayoutGrid renders its SVG with native styles. */
export const LuLayoutGrid: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuLayoutGrid,
)
/** LuLayoutList renders its SVG with native styles. */
export const LuLayoutList: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuLayoutList,
)
/** LuLink renders its SVG with native styles. */
export const LuLink: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuLink)
/** LuList renders its SVG with native styles. */
export const LuList: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuList)
/** LuListChecks renders its SVG with native styles. */
export const LuListChecks: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuListChecks,
)
/** LuListFilter renders its SVG with native styles. */
export const LuListFilter: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuListFilter,
)
/** LuListOrdered renders its SVG with native styles. */
export const LuListOrdered: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuListOrdered,
)
/** LuListTodo renders its SVG with native styles. */
export const LuListTodo: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuListTodo,
)
/** LuLoader renders its SVG with native styles. */
export const LuLoader: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuLoader,
)
/** LuLoaderCircle renders its SVG with native styles. */
export const LuLoaderCircle: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuLoaderCircle,
)
/** LuLocateFixed renders its SVG with native styles. */
export const LuLocateFixed: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuLocateFixed,
)
/** LuLock renders its SVG with native styles. */
export const LuLock: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuLock)
/** LuLockKeyhole renders its SVG with native styles. */
export const LuLockKeyhole: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuLockKeyhole,
)
/** LuLockOpen renders its SVG with native styles. */
export const LuLockOpen: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuLockOpen,
)
/** LuLogIn renders its SVG with native styles. */
export const LuLogIn: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuLogIn)
/** LuLogOut renders its SVG with native styles. */
export const LuLogOut: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuLogOut,
)
/** LuMail renders its SVG with native styles. */
export const LuMail: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuMail)
/** LuMaximize renders its SVG with native styles. */
export const LuMaximize: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuMaximize,
)
/** LuMaximize2 renders its SVG with native styles. */
export const LuMaximize2: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuMaximize2,
)
/** LuMenu renders its SVG with native styles. */
export const LuMenu: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuMenu)
/** LuMerge renders its SVG with native styles. */
export const LuMerge: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuMerge)
/** LuMessageCircle renders its SVG with native styles. */
export const LuMessageCircle: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuMessageCircle,
)
/** LuMessageSquare renders its SVG with native styles. */
export const LuMessageSquare: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuMessageSquare,
)
/** LuMessageSquareText renders its SVG with native styles. */
export const LuMessageSquareText: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuMessageSquareText,
)
/** LuMinus renders its SVG with native styles. */
export const LuMinus: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuMinus)
/** LuMonitor renders its SVG with native styles. */
export const LuMonitor: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuMonitor,
)
/** LuMonitorDown renders its SVG with native styles. */
export const LuMonitorDown: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuMonitorDown,
)
/** LuMoon renders its SVG with native styles. */
export const LuMoon: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuMoon)
/** LuMousePointer renders its SVG with native styles. */
export const LuMousePointer: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuMousePointer,
)
/** LuMousePointer2 renders its SVG with native styles. */
export const LuMousePointer2: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuMousePointer2,
)
/** LuMousePointerClick renders its SVG with native styles. */
export const LuMousePointerClick: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuMousePointerClick,
)
/** LuMoveRight renders its SVG with native styles. */
export const LuMoveRight: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuMoveRight,
)
/** LuMusic renders its SVG with native styles. */
export const LuMusic: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuMusic)
/** LuNetwork renders its SVG with native styles. */
export const LuNetwork: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuNetwork,
)
/** LuNotebookPen renders its SVG with native styles. */
export const LuNotebookPen: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuNotebookPen,
)
/** LuPackage renders its SVG with native styles. */
export const LuPackage: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuPackage,
)
/** LuPaintbrush renders its SVG with native styles. */
export const LuPaintbrush: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuPaintbrush,
)
/** LuPanelTop renders its SVG with native styles. */
export const LuPanelTop: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuPanelTop,
)
/** LuPanelTopOpen renders its SVG with native styles. */
export const LuPanelTopOpen: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuPanelTopOpen,
)
/** LuPenLine renders its SVG with native styles. */
export const LuPenLine: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuPenLine,
)
/** LuPencil renders its SVG with native styles. */
export const LuPencil: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuPencil,
)
/** LuPersonStanding renders its SVG with native styles. */
export const LuPersonStanding: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuPersonStanding,
)
/** LuPinOff renders its SVG with native styles. */
export const LuPinOff: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuPinOff,
)
/** LuPlay renders its SVG with native styles. */
export const LuPlay: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuPlay)
/** LuPlugZap renders its SVG with native styles. */
export const LuPlugZap: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuPlugZap,
)
/** LuPlus renders its SVG with native styles. */
export const LuPlus: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuPlus)
/** LuPuzzle renders its SVG with native styles. */
export const LuPuzzle: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuPuzzle,
)
/** LuQrCode renders its SVG with native styles. */
export const LuQrCode: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuQrCode,
)
/** LuRadar renders its SVG with native styles. */
export const LuRadar: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuRadar)
/** LuRedo2 renders its SVG with native styles. */
export const LuRedo2: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuRedo2)
/** LuRefreshCcw renders its SVG with native styles. */
export const LuRefreshCcw: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuRefreshCcw,
)
/** LuRefreshCw renders its SVG with native styles. */
export const LuRefreshCw: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuRefreshCw,
)
/** LuReplace renders its SVG with native styles. */
export const LuReplace: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuReplace,
)
/** LuRocket renders its SVG with native styles. */
export const LuRocket: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuRocket,
)
/** LuRotateCcw renders its SVG with native styles. */
export const LuRotateCcw: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuRotateCcw,
)
/** LuRotateCw renders its SVG with native styles. */
export const LuRotateCw: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuRotateCw,
)
/** LuRoute renders its SVG with native styles. */
export const LuRoute: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuRoute)
/** LuRows3 renders its SVG with native styles. */
export const LuRows3: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuRows3)
/** LuSave renders its SVG with native styles. */
export const LuSave: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuSave)
/** LuScale renders its SVG with native styles. */
export const LuScale: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuScale)
/** LuScan renders its SVG with native styles. */
export const LuScan: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuScan)
/** LuSearch renders its SVG with native styles. */
export const LuSearch: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuSearch,
)
/** LuSend renders its SVG with native styles. */
export const LuSend: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuSend)
/** LuSendHorizontal renders its SVG with native styles. */
export const LuSendHorizontal: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuSendHorizontal,
)
/** LuServer renders its SVG with native styles. */
export const LuServer: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuServer,
)
/** LuSettings renders its SVG with native styles. */
export const LuSettings: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuSettings,
)
/** LuSettings2 renders its SVG with native styles. */
export const LuSettings2: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuSettings2,
)
/** LuShield renders its SVG with native styles. */
export const LuShield: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuShield,
)
/** LuShieldAlert renders its SVG with native styles. */
export const LuShieldAlert: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuShieldAlert,
)
/** LuShieldCheck renders its SVG with native styles. */
export const LuShieldCheck: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuShieldCheck,
)
/** LuShieldQuestion renders its SVG with native styles. */
export const LuShieldQuestion: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuShieldQuestion,
)
/** LuSiren renders its SVG with native styles. */
export const LuSiren: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuSiren)
/** LuSkipForward renders its SVG with native styles. */
export const LuSkipForward: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuSkipForward,
)
/** LuSlidersHorizontal renders its SVG with native styles. */
export const LuSlidersHorizontal: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuSlidersHorizontal,
)
/** LuSmartphone renders its SVG with native styles. */
export const LuSmartphone: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuSmartphone,
)
/** LuSparkles renders its SVG with native styles. */
export const LuSparkles: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuSparkles,
)
/** LuSquare renders its SVG with native styles. */
export const LuSquare: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuSquare,
)
/** LuSquareCheck renders its SVG with native styles. */
export const LuSquareCheck: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuSquareCheck,
)
/** LuSquarePlus renders its SVG with native styles. */
export const LuSquarePlus: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuSquarePlus,
)
/** LuSquareTerminal renders its SVG with native styles. */
export const LuSquareTerminal: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuSquareTerminal,
)
/** LuStar renders its SVG with native styles. */
export const LuStar: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuStar)
/** LuStarOff renders its SVG with native styles. */
export const LuStarOff: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuStarOff,
)
/** LuStrikethrough renders its SVG with native styles. */
export const LuStrikethrough: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuStrikethrough,
)
/** LuTable renders its SVG with native styles. */
export const LuTable: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuTable)
/** LuTableProperties renders its SVG with native styles. */
export const LuTableProperties: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuTableProperties,
)
/** LuTag renders its SVG with native styles. */
export const LuTag: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuTag)
/** LuTerminal renders its SVG with native styles. */
export const LuTerminal: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuTerminal,
)
/** LuTimer renders its SVG with native styles. */
export const LuTimer: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuTimer)
/** LuTimerOff renders its SVG with native styles. */
export const LuTimerOff: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuTimerOff,
)
/** LuTrash renders its SVG with native styles. */
export const LuTrash: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuTrash)
/** LuTrash2 renders its SVG with native styles. */
export const LuTrash2: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuTrash2,
)
/** LuTriangleAlert renders its SVG with native styles. */
export const LuTriangleAlert: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuTriangleAlert,
)
/** LuTv renders its SVG with native styles. */
export const LuTv: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuTv)
/** LuType renders its SVG with native styles. */
export const LuType: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuType)
/** LuUndo2 renders its SVG with native styles. */
export const LuUndo2: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuUndo2)
/** LuUnlink renders its SVG with native styles. */
export const LuUnlink: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuUnlink,
)
/** LuUpload renders its SVG with native styles. */
export const LuUpload: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuUpload,
)
/** LuUser renders its SVG with native styles. */
export const LuUser: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuUser)
/** LuUserCog renders its SVG with native styles. */
export const LuUserCog: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuUserCog,
)
/** LuUserMinus renders its SVG with native styles. */
export const LuUserMinus: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuUserMinus,
)
/** LuUserPlus renders its SVG with native styles. */
export const LuUserPlus: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuUserPlus,
)
/** LuUserX renders its SVG with native styles. */
export const LuUserX: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuUserX)
/** LuUsers renders its SVG with native styles. */
export const LuUsers: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuUsers)
/** LuVideo renders its SVG with native styles. */
export const LuVideo: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuVideo)
/** LuWandSparkles renders its SVG with native styles. */
export const LuWandSparkles: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuWandSparkles,
)
/** LuWifi renders its SVG with native styles. */
export const LuWifi: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuWifi)
/** LuWifiOff renders its SVG with native styles. */
export const LuWifiOff: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuWifiOff,
)
/** LuWrench renders its SVG with native styles. */
export const LuWrench: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuWrench,
)
/** LuX renders its SVG with native styles. */
export const LuX: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuX)
/** LuZap renders its SVG with native styles. */
export const LuZap: IconType = /*#__PURE__*/ createStyledIcon(iconslu.LuZap)
/** LuZoomIn renders its SVG with native styles. */
export const LuZoomIn: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuZoomIn,
)
/** LuZoomOut renders its SVG with native styles. */
export const LuZoomOut: IconType = /*#__PURE__*/ createStyledIcon(
  iconslu.LuZoomOut,
)

import * as iconsri from 'react-icons/ri'
/** RiErrorWarningLine renders its SVG with native styles. */
export const RiErrorWarningLine: IconType = /*#__PURE__*/ createStyledIcon(
  iconsri.RiErrorWarningLine,
)

import * as iconsrx from 'react-icons/rx'
/** RxArrowRight renders its SVG with native styles. */
export const RxArrowRight: IconType = /*#__PURE__*/ createStyledIcon(
  iconsrx.RxArrowRight,
)
/** RxChevronRight renders its SVG with native styles. */
export const RxChevronRight: IconType = /*#__PURE__*/ createStyledIcon(
  iconsrx.RxChevronRight,
)
/** RxCube renders its SVG with native styles. */
export const RxCube: IconType = /*#__PURE__*/ createStyledIcon(iconsrx.RxCube)
/** RxPerson renders its SVG with native styles. */
export const RxPerson: IconType = /*#__PURE__*/ createStyledIcon(
  iconsrx.RxPerson,
)
