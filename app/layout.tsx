import type { Metadata } from "next";
import "./globals.css";
export const metadata: Metadata={title:"Signal — News, clarified",description:"Distinct, source-forward stories for what matters now.",icons:{icon:"/favicon.svg",shortcut:"/favicon.svg"}};
export default function RootLayout({children}:Readonly<{children:React.ReactNode}>){return <html lang="en"><body>{children}</body></html>}
