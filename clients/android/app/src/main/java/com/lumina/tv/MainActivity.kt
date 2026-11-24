package com.lumina.tv

import android.annotation.SuppressLint
import android.app.Activity
import android.app.UiModeManager
import android.content.Context
import android.content.pm.ActivityInfo
import android.content.res.Configuration
import android.graphics.Color
import android.os.Bundle
import android.view.Gravity
import android.view.KeyEvent
import android.view.View
import android.view.ViewGroup
import android.view.WindowManager
import android.view.inputmethod.EditorInfo
import android.webkit.*
import android.widget.*

/**
 * Cliente Lumina para Android / Android TV / Fire TV.
 * Es un WebView a pantalla completa que carga la web de tu servidor Lumina.
 * Tecla amarilla / Menú: cambiar la dirección del servidor.
 */
class MainActivity : Activity() {
    private lateinit var root: FrameLayout
    private lateinit var web: WebView
    private lateinit var setup: LinearLayout
    private lateinit var input: EditText
    private lateinit var error: TextView
    private var custom: View? = null
    private var customCb: WebChromeClient.CustomViewCallback? = null
    private val prefs by lazy { getSharedPreferences("lumina", MODE_PRIVATE) }

    private val isTv: Boolean by lazy {
        val ui = getSystemService(Context.UI_MODE_SERVICE) as UiModeManager
        ui.currentModeType == Configuration.UI_MODE_TYPE_TELEVISION ||
            packageManager.hasSystemFeature("amazon.hardware.fire_tv") ||
            packageManager.hasSystemFeature("android.software.leanback")
    }

    override fun onCreate(b: Bundle?) {
        super.onCreate(b)
        // tele: siempre horizontal; móvil y tablet: giran libremente (la web tiene vista vertical)
        requestedOrientation = if (isTv) ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE else ActivityInfo.SCREEN_ORIENTATION_FULL_SENSOR
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        root = FrameLayout(this).apply { setBackgroundColor(Color.parseColor("#0F171E")) }
        setContentView(root)
        buildWeb()
        buildSetup()
        hideSystemBars()
        val saved = prefs.getString("server", null)
        if (saved.isNullOrBlank()) showSetup(null) else load(saved)
    }

    @SuppressLint("SetJavaScriptEnabled")
    private fun buildWeb() {
        web = WebView(this)
        web.setBackgroundColor(Color.parseColor("#0F171E"))
        web.settings.apply {
            javaScriptEnabled = true
            domStorageEnabled = true
            mediaPlaybackRequiresUserGesture = false
            mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
            // "LumTV" activa el modo mando (foco, teclas) en la web
            userAgentString = userAgentString + if (isTv) " LumTV" else " LumApp"
        }
        web.webViewClient = object : WebViewClient() {
            override fun onReceivedError(v: WebView, r: WebResourceRequest, e: WebResourceError) {
                if (r.isForMainFrame) showSetup("No se pudo conectar con el servidor (${if (android.os.Build.VERSION.SDK_INT >= 23) e.description else e.errorCode}). Revisa la dirección.")
            }
        }
        web.webChromeClient = object : WebChromeClient() {
            override fun onShowCustomView(view: View, cb: CustomViewCallback) {
                custom = view; customCb = cb
                root.addView(view, ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
            }
            override fun onHideCustomView() {
                custom?.let { root.removeView(it) }; custom = null; customCb?.onCustomViewHidden(); customCb = null
            }
        }
        root.addView(web, ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
    }

    private fun buildSetup() {
        fun dp(v: Int) = (v * resources.displayMetrics.density).toInt()
        setup = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL; gravity = Gravity.CENTER
            setBackgroundColor(Color.parseColor("#0F171E")); setPadding(dp(40), dp(24), dp(40), dp(24))
            visibility = View.GONE
        }
        fun tv(t: String, size: Float, color: String) = TextView(this).apply {
            text = t; textSize = size; setTextColor(Color.parseColor(color)); gravity = Gravity.CENTER
        }
        input = EditText(this).apply {
            hint = "192.168.1.50:8780"; setSingleLine(); setTextColor(Color.WHITE); setHintTextColor(Color.GRAY)
            imeOptions = EditorInfo.IME_ACTION_GO; textSize = 22f; gravity = Gravity.CENTER
            setOnEditorActionListener { _, _, _ -> connect(); true }
        }
        error = tv("", 16f, "#FF5C6C")
        val go = Button(this).apply { text = "Conectar"; setOnClickListener { connect() } }
        setup.addView(tv("Lumina", 40f, "#FFFFFF"))
        setup.addView(tv("Dirección de tu servidor Lumina", 18f, "#8FA2B1"))
        setup.addView(input, LinearLayout.LayoutParams(dp(420), ViewGroup.LayoutParams.WRAP_CONTENT).apply { topMargin = dp(18) })
        setup.addView(error, LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply { topMargin = dp(8) })
        setup.addView(go, LinearLayout.LayoutParams(dp(220), ViewGroup.LayoutParams.WRAP_CONTENT).apply { topMargin = dp(12) })
        root.addView(setup, ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
    }

    private fun norm(u: String): String {
        var s = u.trim().trimEnd('/')
        if (s.isEmpty()) return ""
        if (!s.startsWith("http://", true) && !s.startsWith("https://", true)) s = "http://$s"
        return s
    }

    private fun connect() {
        val u = norm(input.text.toString())
        if (u.isEmpty()) { error.text = "Escribe una dirección"; return }
        prefs.edit().putString("server", u).apply()
        load(u)
    }

    private fun load(u: String) {
        setup.visibility = View.GONE
        web.visibility = View.VISIBLE
        web.loadUrl("$u/" + if (isTv) "?tv=1" else "")
        web.requestFocus()
    }

    private fun showSetup(msg: String?) {
        web.visibility = View.INVISIBLE
        input.setText(prefs.getString("server", ""))
        error.text = msg ?: ""
        setup.visibility = View.VISIBLE
        input.requestFocus()
    }

    private fun hideSystemBars() {
        @Suppress("DEPRECATION")
        window.decorView.systemUiVisibility = (View.SYSTEM_UI_FLAG_FULLSCREEN or View.SYSTEM_UI_FLAG_HIDE_NAVIGATION or
            View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY or View.SYSTEM_UI_FLAG_LAYOUT_STABLE or
            View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN or View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION)
    }

    override fun onWindowFocusChanged(hasFocus: Boolean) { super.onWindowFocusChanged(hasFocus); if (hasFocus) hideSystemBars() }
    override fun onPause() { super.onPause(); web.onPause() }
    override fun onResume() { super.onResume(); web.onResume() }
    override fun onDestroy() { web.destroy(); super.onDestroy() }

    override fun onKeyDown(code: Int, e: KeyEvent): Boolean {
        when (code) {
            KeyEvent.KEYCODE_PROG_YELLOW, KeyEvent.KEYCODE_MENU -> { showSetup(null); return true }
            KeyEvent.KEYCODE_BACK -> {
                if (setup.visibility == View.VISIBLE) {
                    if (!prefs.getString("server", "").isNullOrBlank()) load(prefs.getString("server", "")!!) else finish()
                    return true
                }
                if (custom != null) { web.webChromeClient?.onHideCustomView(); return true }
                web.evaluateJavascript("(window.LuminaBack&&window.LuminaBack())?'1':'0'") { r ->
                    if (r != "\"1\"") finish()
                }
                return true
            }
        }
        return super.onKeyDown(code, e)
    }
}
